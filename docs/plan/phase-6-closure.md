# Cierre de la Fase 6 — Disponibilidad y reservas

Fecha: 2026-11-24 · Estado: **✅ Aprobada por el usuario (2026-11-24)**

## 1. Entregables

### Lo que hace hoy la plataforma

1. **El cliente ve las horas libres** de un barbero para un servicio en un día. Se calculan con
   su horario, descansos, ausencias, citas, la duración del servicio y las reglas del negocio
   (intervalo, preparación, antelación, horizonte), en la zona horaria del negocio y con los
   cambios de horario de verano resueltos.
2. **El cliente reserva desde la app.** Necesita una cuenta con email verificado y celular, que
   es obligatorio desde el registro. **Nunca hay dos citas a la misma hora:** se bloquea la
   agenda del barbero, se revalida la hora y la base de datos rechaza cualquier cruce. Repetir la
   petición no duplica la cita.
3. **El barbero agenda a mano en un solo paso:** escribe nombre, celular y correo (opcional) del
   cliente. Si ese celular ya es cliente del negocio se reutiliza; si no, se crea. La cita nace
   confirmada.
4. **Agenda:**
   - el barbero ve la suya y el responsable la de todos, por día, semana, mes o año;
   - el cliente ve sus citas de todos los negocios.
5. **Cambios de la cita:**
   - confirmar, cancelar y reprogramar (el cliente, hasta 2 h antes);
   - pasar a otro barbero (solo el responsable);
   - iniciar, completar y "no vino", que libera el horario al instante;
   - todo queda en el historial de la cita.
6. **Coherencia:** una ausencia o un cambio de horario que choca con citas se rechaza mostrando
   la lista. Un barbero retirado deja sus citas "por reasignar" y el responsable las pasa a otro.

Validado en QA de punta a punta con el script E2E (**194/194** tras T6.7) y con la colección de
Postman (`docs/testing/postman/`) verificada con Newman.

### Tareas

| Tarea | Entregable | Estado | Evidencia |
|---|---|---|---|
| T6.1 | Motor de disponibilidad: función pura, DST resuelto, igual a una referencia minuto a minuto | ✅ | PR #48 |
| T6.2 | Disponibilidad pública por barbero, servicio y fecha | ✅ | PR #49, E2E 142/143 (falla del script) |
| T6.3 | Reserva del cliente: celular, `Idempotency-Key`, bloqueo + exclusion constraint, snapshot | ✅ | PR #50, E2E 157/157 |
| T6.4 | Celular obligatorio en el registro; clientes del negocio; cita manual rápida | ✅ | PR #51, E2E 166/166 |
| T6.5 | Agenda (día, semana, mes, año) y "mis citas" de todos los negocios | ✅ | PR #52, E2E 174/174 |
| T6.6 | Cambios de estado, reprogramación, reasignación, historial | ✅ | PR #53, E2E 185/185 |
| T6.7 | Coherencia: ausencias y horario contra citas; citas por reasignar | ✅ | PR #54, E2E 194/194 |
| T6.8 | Las 7 pruebas de concurrencia del diseño completas, cobertura, este documento | ✅ | `feature/t6.8-phase6-closure` |

Módulo nuevo: `scheduling`. Contratos nuevos entre módulos:
- `businesses.PublicBooking`;
- `catalog.Booking`;
- `professionals.Agenda` y `professionals.Bookable`;
- `identity.Customers`;
- `scheduling.AgendaGuard`.

Migraciones 00016–00018. API: **81 rutas, todas documentadas en `api/openapi.yaml`** (eran 60 al
cerrar la Fase 5).

## 2. Las 7 pruebas de concurrencia obligatorias (`availability-engine.md` §7)

Todas con PostgreSQL real (testcontainers). En CI la integración corre con `-race`.

| Caso del diseño | Resultado | Prueba |
|---|---|---|
| 50 reservas del mismo horario | **1 éxito, 49 × 409** | `TestNoDoubleBookingUnderConcurrency` |
| Reservas parcialmente solapadas en paralelo | 1 éxito | `TestNoDoubleBookingUnderConcurrency` |
| Reserva ∥ ausencia en el mismo rango | Exactamente una gana (10 rondas) | `TestAgendaCoherenceOnPostgres` |
| Cancelación ∥ reprogramación | Estado coherente; versión +1 por cambio exitoso (8 rondas) | `TestAppointmentChangesOnPostgres` |
| Confirmaciones concurrentes | 12 respuestas OK, 1 transición, 1 evento | `TestConcurrentConfirmationsOnPostgres` (T6.8) |
| Misma `Idempotency-Key` en paralelo | 1 cita, todas las respuestas con su id | `TestNoDoubleBookingUnderConcurrency` |
| Reserva en el cambio de horario de verano | Instantes UTC correctos, sin duplicados; las dos 01:00 de otoño reservables | `TestBookingAcrossDST` (T6.8) |

**Mutaciones** (se quita una protección y se comprueba que la prueba falla):

| Protección quitada | Resultado |
|---|---|
| Sin bloqueo del barbero al reservar | La regla de la base de datos sola impide la doble reserva |
| Sin la regla de la base de datos | El bloqueo solo impide la doble reserva |
| Sin ninguna de las dos | Detectado: 2 y 16 reservas dobles |
| Sin orden fijo de bloqueos al reasignar | Detectado: deadlock en la 1.ª ronda |
| Ausencias sin bloqueo | Detectado: reserva y ausencia encimadas en la 1.ª ronda |

## 3. Pruebas

| Tipo | Estado | Evidencia (`docs/testing/test-results.md`) |
|---|---|---|
| Unitarias | ✅ | Cobertura: scheduling dominio 99,4 %, aplicación 85,7 %; professionals dominio 99,2 %, aplicación 88,1 %; catálogo aplicación 93,7 %; negocios aplicación 90,2 %; identity aplicación 84,3 % |
| Integración (PostgreSQL real) | ✅ | Reserva, cita manual, agenda, cambios y coherencia con los módulos reales; las 7 de concurrencia |
| Contrato OpenAPI | ✅ | 81 rutas ↔ contrato en ambas direcciones; cada operación nueva validada con respuestas reales |
| Seguridad (`tests/security`) | ✅ | **Aislamiento entre negocios en 39 operaciones** (404 idéntico, sin cambios, denegación auditada), **24 con ids ajenos** dentro del propio negocio (IDOR), **matriz de credenciales inválidas en las 66 operaciones autenticadas**; las tres pruebas se generan desde el contrato y cubren solas las rutas nuevas |
| E2E en QA (HTTPS público) | ✅ | **194/194** tras T6.7; se repite tras desplegar T6.8 |
| Postman + Newman | ✅ | 53 peticiones, 93 comprobaciones, 0 fallos (2 corridas seguidas) |
| Estático y dependencias | ✅ | golangci-lint (depguard por módulo), sqlc vet/diff, govulncheck, gitleaks, Trivy, shellcheck |

Nota: los handlers HTTP no tienen pruebas en su propio paquete; se ejercitan con las pruebas de
contrato, de seguridad, el E2E y Postman.

## 4. Checklist de seguridad de la fase

| Control | Verificación |
|---|---|
| Cero doble reserva (ADR-0004) | Bloqueo por barbero + exclusion constraint; las 7 pruebas de concurrencia; mutaciones |
| Otro negocio → 404 idéntico; ids ajenos en el propio negocio → 404 | `TestEveryBusinessRouteIsTenantIsolated` (guiada por el contrato) |
| El barbero solo actúa en su agenda; reasignar solo el responsable | Pruebas de aplicación y contrato (403) |
| Un cliente solo ve y cambia sus citas | Cita ajena → 404 idéntico a inexistente |
| Disponibilidad y catálogo públicos sin datos internos | `TestPublicSearchExposesOnlyBookableBusinesses` |
| Las rutas autenticadas rechazan toda credencial inválida | Matriz desde OpenAPI (66 operaciones) |
| Autorización antes de cualquier validación | Detectado y corregido en T6.4 por la suite de aislamiento |
| Datos personales: celular y correo de clientes solo para el personal del negocio | Agenda y clientes solo con rol en el negocio |

## 5. Decisiones del usuario incorporadas

- El cliente elige barbero (obligatorio). Sin auto-cancelación de citas pendientes.
- Email verificado y **celular obligatorio al crear la cuenta**.
- Ausencias como bloqueos. Ausencia o cambio de horario sobre citas: rechazo con la lista.
- Cita manual **rápida**: nombre y celular obligatorios, correo opcional; el cliente puede estar
  o no registrado.
- Agenda por día, semana, mes y año; el responsable ve todas; el cliente, las suyas de todos los
  negocios.
- Reprogramar el negocio: pendiente si el cliente tiene cuenta, confirmada si no; motivo
  obligatorio para que el negocio cancele; las citas pasadas solo se completan o se marcan
  "no vino"; **"no vino" libera el horario**.
- Retirar a un barbero con citas: permitido; quedan por reasignar.

## 6. Riesgos aceptados y deuda

| Tema | Estado |
|---|---|
| Notificaciones (confirmación, recordatorios, cambios) | Fase 7: los eventos ya quedan en `appointment_events` |
| P40: invitar a la app al cliente agendado a mano | Decidido 2026-11-24: WhatsApp con el enlace de descarga apenas el barbero lo agenda (Fase 7) |
| Las claves de idempotencia no se purgan | Purga programada (River) en la Fase 12 |
| DEV local con datos viejos (la migración 6 no aplica) | Recrear el volumen DEV cuando el usuario lo autorice |
| Datos de prueba en QA: cada corrida E2E y de Postman deja negocios, clientes y citas sintéticos | Aceptado en QA |
| P37 (sin protección de ramas), P38 (backups en el mismo disco) | Aceptados en QA |

## 7. Aprobación

- [x] Usuario aprueba el cierre de la Fase 6 (2026-11-24).
