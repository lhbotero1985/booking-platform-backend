# Siguiente tarea

## Última tarea completada
T6.6 — Cambios de estado de la cita (2026-11-24), en `feature/t6.6-appointment-changes` (commit
y push; PR y merge pendientes de autorización).

**Fase 6 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T6.7 — Coherencia de la agenda

- Ausencias y cambios de horario toman el bloqueo del barbero y se rechazan si chocan con citas
  activas, devolviendo la lista de citas en conflicto (P11).
- Retirar o desactivar un barbero con citas futuras: permitido; sus citas quedan "por reasignar"
  y el responsable las pasa a otro barbero (ya existe la reasignación de T6.6).
- Prueba: reserva y ausencia en paralelo sobre el mismo rango → solo una gana.
