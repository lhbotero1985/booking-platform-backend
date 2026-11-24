# Siguiente tarea

## Última tarea completada
T5.8 — Suite de aislamiento, E2E y cierre de la Fase 5 (2026-11-24), en
`feature/t5.8-phase5-closure` (commit y push; PR y merge pendientes de autorización).

**Fase 5 aprobada por el usuario (2026-11-24):** ver `docs/plan/phase-5-closure.md`.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

Fase 6 — Motor de reservas: proponer el desglose en tareas (disponibilidad a partir de horario,
descansos, ausencias y duración del servicio; creación de citas sin doble reserva según ADR-0004;
citas manuales del barbero; P11 ausencia sobre citas existentes) para aprobación del usuario.
