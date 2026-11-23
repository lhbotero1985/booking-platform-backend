# Siguiente tarea

## Última tarea completada
T4.1 — Geografía (2026-11-23), en `feature/t4.1-geography` (commit y push; PR y merge pendientes
de autorización). Al desplegar: otorgar `PLATFORM_ADMIN` a la cuenta que indique el usuario y
repetir `scripts/e2e-qa.sh`.

**Fase 4 en curso** (desglose aprobado 2026-11-23, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T4.2 — Dominio y persistencia de negocios

- Migración `businesses` (slug citext único, estado, contacto, ubicación país/división/ciudad +
  `location geography(Point)` opcional, `time_zone`, `currency`, política de reservas, avisos).
- FK `memberships.business_id → businesses` (RESTRICT); `audit_log.business_id` sin FK.
- Invariante pendiente de T4.1: no desactivar una ciudad con negocios activos.
- Teléfono con `github.com/nyaruka/phonenumbers` (nueva dependencia, govulncheck).
