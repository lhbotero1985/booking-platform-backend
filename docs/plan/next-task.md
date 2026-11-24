# Siguiente tarea

## Última tarea completada
T6.3 — Reserva del cliente (2026-11-24), en `feature/t6.3-customer-booking` (commit y push; PR
y merge pendientes de autorización).

**Fase 6 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T6.4 — Clientes del negocio y citas manuales

- Clientes del negocio sin cuenta (nombre + teléfono), por negocio.
- Cita manual: el responsable para cualquier barbero; el barbero solo en su agenda (403 en otra);
  puede nacer `CONFIRMED`; mismas garantías de concurrencia (lock + constraint).
