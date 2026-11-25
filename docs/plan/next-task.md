# Siguiente tarea

## Última tarea completada
T6.7 — Coherencia de la agenda (2026-11-24), en `feature/t6.7-agenda-coherence` (commit y push;
PR y merge pendientes de autorización).

**Fase 6 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T6.8 — Cierre de la Fase 6

- Revisar la suite de aislamiento y la matriz de credenciales (ya guiadas por el contrato).
- Las 7 pruebas de concurrencia de `availability-engine.md` §7, comprobando cuáles ya existen.
- E2E completo en QA y `docs/plan/phase-6-closure.md` para aprobación del usuario.
