# Siguiente tarea

## Última tarea completada
T5.6 — Servicios que presta cada profesional (2026-11-24), en
`feature/t5.6-professional-services` (commit y push; PR y merge pendientes de autorización).

**Fase 5 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T5.7 — Vista pública de profesionales y servicios

- En la ficha pública del negocio (`GET /public/businesses/{slug}`, sin login): servicios
  activos (nombre, descripción, duración, precio, categoría) y profesionales reservables
  (nombre visible, descripción) con los servicios que presta cada uno.
- La búsqueda pública solo muestra negocios con al menos un servicio activo y un profesional
  reservable que lo preste.
- Sin datos internos (ids de usuario, emails, estado de membresía, auditoría).
