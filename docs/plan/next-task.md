# Siguiente tarea

## Última tarea completada
T5.5 — Catálogo de servicios (2026-11-24), en `feature/t5.5-catalog` (commit y push; PR y merge
pendientes de autorización).

**Fase 5 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T5.6 — Servicios que presta cada profesional

- Tabla `service_professionals(business_id, service_id, professional_id)` con FK compuestas a
  `services` y `professionals` del mismo negocio.
- El OWNER asigna qué servicios presta cada barbero (reemplazo de la lista completa en una
  transacción); el equipo lo consulta.
- Solo servicios y profesionales del mismo negocio; otro negocio 404; auditoría.
