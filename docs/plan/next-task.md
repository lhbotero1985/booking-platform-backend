# Siguiente tarea

## Última tarea completada
T4.6 — Búsqueda pública de negocios (2026-11-24), en `feature/t4.6-public-search` (commit y
push; PR y merge pendientes de autorización).

**Fase 4 en curso** (desglose aprobado 2026-11-23 y ajustado 2026-11-24, ADR-0018).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T4.7 — Cierre de la Fase 4

- Suite transversal de aislamiento entre negocios (IDOR) en `tests/security` que recorra todas
  las rutas `/business/{id}/…` con un usuario de otro negocio (404 idéntico).
- Revisión del contrato OpenAPI completo, E2E final en QA, `docs/plan/phase-4-closure.md`.
- Aprobación del usuario.
