# Siguiente tarea

## Última tarea completada
T4.2 — Dominio y persistencia de negocios (2026-11-24), en `feature/t4.2-businesses` (commit y
push; PR y merge pendientes de autorización).

**Fase 4 en curso** (desglose aprobado 2026-11-23, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T4.3 — Alta de negocios por el administrador

- `POST /api/v1/admin/businesses` (PLATFORM_ADMIN): nombre, teléfono, `cityId`, dirección,
  coordenadas opcionales, email del OWNER (usuario registrado y verificado); slug automático;
  zona horaria y moneda desde la ciudad/país; negocio + membresía OWNER + auditoría en una
  transacción (identity expone la creación de la membresía por su contract).
- `POST /api/v1/admin/businesses/{id}/suspend|activate`; `GET /api/v1/admin/businesses/{id}`.
- Decisión pendiente del usuario: ¿el negocio nace `ACTIVE` (visible) o pendiente hasta que el
  administrador lo active (útil mientras se configuran profesionales y servicios, Fase 5)?
