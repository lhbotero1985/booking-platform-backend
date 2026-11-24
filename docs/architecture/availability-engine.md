# Motor de disponibilidad y concurrencia

Estado: **Propuesta — Fase 0.** Componente crítico. ADR-0004 y ADR-0005.

## 1. Conceptos

- **Duración (D)**: minutos reales del servicio (snapshot). No se redondea.
- **Intervalo de inicio (I)**: separación entre horas de inicio ofrecidas (5/10/15/20, def. 10).
- **Preparación (B)**: minutos de buffer después de cada cita (0–60).
- **Ventana laboral**: rango de trabajo de un día, ya convertido a instantes UTC.
- **Ocupación de una cita**: `[inicio, fin + B)`.

## 2. Entradas y salida

```
Available(input) -> []Slot
input:
  businessTZ, policy{I, B, minLead, maxAdvanceDays}
  service{D}, professional{workingHours, breaks, absences}
  blocks, activeAppointments (rangos ocupados)
  localDate (YYYY-MM-DD en la zona del negocio), now (UTC)
Slot: { startAt (UTC), endAt (UTC), startLocal (con offset) }
```

El cálculo es una **función pura del dominio** (`scheduling/domain/availability`), sin acceso a
BD ni a reloj global: altamente testeable con tablas de casos.

## 3. Algoritmo

1. Validar fecha: `hoyLocal ≤ localDate ≤ hoyLocal + maxAdvanceDays`; profesional activo y
   habilitado para el servicio; negocio activo.
2. Construir ventanas laborales del día: para cada `LocalTimeRange` del día de la semana,
   `time.Date(y, m, d, hh, mm, 0, 0, loc)` para inicio y fin → instantes UTC.
3. Restar de las ventanas: descansos del día (convertidos igual), ausencias y bloqueos que
   intersecten. Resultado: **segmentos libres** `[a, b)` (sin considerar aún citas).
4. Para cada segmento libre, generar candidatos `t = a + k·I` (paso en tiempo absoluto, anclado
   al inicio del rango laboral original para que la grilla sea estable aun si hay descansos).
5. Un candidato `t` es válido si:
   - `[t, t + D) ⊆ [a, b)` — el servicio completo cabe en la ventana libre (la preparación final
     **no** necesita caber antes del cierre).
   - `[t, t + D + B)` no se cruza con la ocupación `[s_i, e_i + B_i)` de ninguna cita activa.
   - `t ≥ now + minLead`.
6. Devolver los slots ordenados, con hora local y offset para que el cliente los muestre sin
   cálculos de zona horaria.

Complejidad: O(ventanas × candidatos × citas) por día y profesional; con agendas de barbería
(< 40 citas/día) es despreciable. Si se necesita, las citas se recorren con dos punteros.

### Ejemplo

Horario 09:00–12:00, I = 10, B = 0, cita existente 09:40–10:20, servicio D = 40:
- 09:00 ✅ (09:00–09:40 toca pero no cruza `[09:40, 10:20)`)
- 09:10 ❌ … 10:10 ❌ (se cruzan con la cita)
- 10:20 ✅ … 11:20 ✅ (11:20–12:00 cabe exacto)
- 11:30 ❌ (terminaría 12:10)

## 4. Zonas horarias y horario de verano (ADR-0005)

- Almacenamiento siempre en UTC (`timestamptz`); la zona IANA del negocio se usa solo para
  interpretar horarios locales y presentar resultados.
- **Hora local inexistente** (salto hacia adelante, p. ej. 02:30 que no existe): se convierte en
  **el instante del salto** (02:30 → 03:00); si un borde de ventana cae en el hueco, la ventana
  conserva solo su parte existente y la conversión nunca retrocede. `time.Date` no especifica
  este caso, por eso la conversión es propia (`scheduling/domain.ResolveLocal`, T6.1). El paso de
  candidatos en tiempo absoluto evita slots duplicados o fantasma.
- **Hora local ambigua** (retroceso, 01:30 ocurre dos veces): se usa la primera ocurrencia para
  bordes; los candidatos en tiempo absoluto cubren la hora repetida sin duplicar instantes.
- Pruebas obligatorias con zonas con y sin DST (`America/Bogota`, `America/Mexico_City`,
  `America/Santiago`, `America/New_York`, `Europe/Madrid`) en las fechas de transición.
- La imagen Docker incluye `tzdata` (o se usa `time/tzdata` embebido) para que el resultado no
  dependa del sistema operativo.

## 5. Modo "cualquier profesional"

**Descartado (usuario, 2026-11-24):** el cliente elige barbero y es obligatorio.

## 6. Concurrencia y prevención de doble reserva (ADR-0004)

La consulta de disponibilidad es **informativa** (sin bloqueos). La garantía está en la reserva:

```text
BEGIN (READ COMMITTED)
  1. SELECT … FROM idempotency_keys WHERE user_id=$u AND key=$k  → si existe, devolver respuesta guardada
  2. SELECT id FROM professionals WHERE id=$p AND business_id=$b FOR UPDATE
        -- serializa todas las escrituras de agenda de ESE profesional
        -- (reservar, reprogramar, crear bloqueo/ausencia también toman este lock)
  3. Cargar horario, descansos, ausencias, bloqueos y citas activas del día
  4. Ejecutar AvailabilityCalculator para el instante solicitado (revalidación)
  5. INSERT INTO appointments …   ← exclusion constraint como red de seguridad
  6. INSERT INTO appointment_events …
  7. River InsertTx(jobs de notificación)
  8. INSERT INTO idempotency_keys …
COMMIT
```

- El `FOR UPDATE` sobre la fila del profesional garantiza que reglas que el constraint no puede
  expresar (bloqueos, ausencias, horario laboral, descansos) también se respeten bajo carrera.
- El **exclusion constraint** garantiza que, incluso con un error de código o un camino que olvide
  el lock, dos citas activas nunca se solapen. `23P01` → `409 SLOT_UNAVAILABLE`.
- Lock de grano fino (por profesional): reservas con profesionales distintos no compiten.
- Las transiciones de estado usan bloqueo optimista (`version`).
- Timeouts: `lock_timeout = 3s`, `statement_timeout = 5s` en la sesión de reserva → 503
  reintentable en lugar de colgar.

## 7. Pruebas obligatorias de concurrencia (Fase 6)

| Caso | Resultado esperado |
|---|---|
| 50 goroutines reservan el mismo slot | Exactamente 1 éxito, 49 × 409 |
| Reservas parcialmente solapadas (09:00 y 09:20, D = 40) en paralelo | 1 éxito |
| Reserva vs creación de bloqueo en paralelo en el mismo rango | Solo una prevalece; nunca ambas |
| Cancelación + reprogramación concurrentes | Estado final consistente; versión incrementada una vez por operación exitosa |
| Confirmaciones repetidas/concurrentes | 1 transición, N respuestas 200, 1 notificación |
| Mismo `Idempotency-Key` en paralelo | 1 cita creada |
| Reserva en hora de cambio DST | Instantes UTC correctos, sin duplicados |

Todas con PostgreSQL real (testcontainers) y `go test -race`.
