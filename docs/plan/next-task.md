# Siguiente tarea

## Última tarea completada
T6.4 — Clientes del negocio y citas manuales (2026-11-24), en
`feature/t6.4-manual-appointments` (commits y push; PR y merge pendientes de autorización).

**Fase 6 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T6.5 — Agenda y "mis citas"

- Agenda del negocio (OWNER: todos los barberos) y del barbero (la suya) por día/semana, en la
  zona del negocio, con cliente, servicio y estado.
- "Mis citas" del cliente (próximas y pasadas) y el detalle de una cita propia.
- Otro negocio → 404; un cliente no ve citas ajenas (404).
