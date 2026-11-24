# Siguiente tarea

## Última tarea completada
T6.2 — Disponibilidad pública (2026-11-24), en `feature/t6.2-public-availability` (commit y
push; PR y merge pendientes de autorización). Colección de Postman para que el usuario la valide
en QA tras el despliegue.

**Fase 6 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T6.3 — Reserva del cliente

- Migración `appointments` (snapshot del servicio, `version`, exclusion constraint sobre
  `tstzrange(start_at, end_at + buffer)` en estados activos), `appointment_events`,
  `idempotency_keys`; teléfono obligatorio del cliente (perfil de identity).
- `POST /api/v1/appointments` con `Idempotency-Key`: lock del profesional, revalidación con el
  motor, inserción; 409 `SLOT_UNAVAILABLE`.
- La disponibilidad descuenta las citas activas.
- Pruebas de concurrencia: 50 en paralelo → 1 éxito; solapadas → 1 éxito; misma clave → 1 cita.
