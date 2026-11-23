# Siguiente tarea

## Última tarea completada
T3.8 — Cierre de la Fase 3 (2026-11-23). **Fase 3 aprobada por el usuario.** El paso `qa` →
`main` se hará al ir a producción (Fase 13).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
(merge commit + tag `vX.Y.Z`, `release-tag.yml`) solo al pasar a producción.

## Próxima tarea: desglose de la Fase 4

Proponer el desglose de la Fase 4 según `roadmap.md` (tareas, criterios de aceptación, decisiones
pendientes) y esperar la aprobación del usuario antes de empezar.
