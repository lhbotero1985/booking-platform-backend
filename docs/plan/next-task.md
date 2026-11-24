# Siguiente tarea

## Última tarea completada
T4.7 — Cierre de la Fase 4 (2026-11-24), en `feature/t4.7-phase-4-closure` (commit y push; PR y
aprobación del cierre pendientes). Ver `docs/plan/phase-4-closure.md`.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Pendiente de autorización

1. Aprobación del cierre de la Fase 4.
2. PR `feature/t4.7-phase-4-closure` → `qa`.

## Después: Fase 5 — Profesionales y servicios

Desglose para aprobación: profesionales (barberos) vinculados a miembros, horarios semanales,
descansos, ausencias, categorías y servicios con precio y duración, servicios por profesional;
la búsqueda pública pasa a exigir al menos un servicio activo (ADR-0018).
