# ADR-0004: Prevención de doble reserva

- Estado: Aceptado (aprobación Fase 0, 2026-11-18) · Fecha: 2026-11-18

## Contexto
Requisito crítico: dos citas activas nunca pueden ocupar el mismo intervalo de un profesional,
incluido el tiempo de preparación, y tampoco pueden invadir bloqueos, ausencias, descansos u
horario no laboral.

## Decisión
Defensa en dos capas dentro de una transacción READ COMMITTED:

1. `SELECT … FROM professionals WHERE id=$1 AND business_id=$2 FOR UPDATE` serializa todas las
   escrituras de agenda de un profesional (reservar, reprogramar, bloquear, ausencias). Dentro
   del lock se revalida la disponibilidad completa.
2. Exclusion constraint sobre `occupied = tstzrange(start_at, end_at + buffer, '[)')`:
   `EXCLUDE USING gist (professional_id WITH =, occupied WITH &&) WHERE (status IN
   ('PENDING_CONFIRMATION','CONFIRMED','IN_PROGRESS'))`. Error `23P01` → HTTP 409.

Transiciones de estado: bloqueo optimista por `version`. Creación: `Idempotency-Key`.

## Alternativas
- SERIALIZABLE: correcto pero con reintentos frecuentes y difícil de razonar.
- Solo constraint: no cubre bloqueos/horarios.
- Advisory locks: equivalentes al `FOR UPDATE`, menos visibles; alternativa si hay contención.
- Tabla de slots pre-generados: rígida ante cambios de intervalo/duración.

## Consecuencias
- (+) Garantía en la base de datos independiente de errores de la aplicación.
- (+) Contención solo por profesional.
- (−) Las reservas de un mismo profesional se serializan (aceptable para el volumen esperado).
- Requiere pruebas de concurrencia con PostgreSQL real antes de aprobar la Fase 6.
