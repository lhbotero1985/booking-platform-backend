# Siguiente tarea

## Última tarea completada
T5.7 — Vista pública de profesionales y servicios (2026-11-24), en
`feature/t5.7-public-catalog` (commit y push; PR y merge pendientes de autorización).

**Fase 5 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T5.8 — Cierre de la Fase 5

- Revisar la suite de aislamiento (todas las rutas `{businessId}` y las públicas nuevas).
- E2E completo en QA con el flujo del responsable: negocio → barberos → horario → ausencia →
  catálogo → asignación → aparece en la búsqueda pública con su catálogo.
- Documento `docs/plan/phase-5-closure.md` para aprobación del usuario.
