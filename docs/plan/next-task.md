# Siguiente tarea

## Última tarea completada
T6.1 — Motor de disponibilidad (2026-11-24), en `feature/t6.1-availability-engine` (commit y
push; PR y merge pendientes de autorización).

**Fase 6 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T6.2 — Disponibilidad pública

- `GET /api/v1/businesses/{slug}/availability?serviceId=&professionalId=&date=YYYY-MM-DD` (sin
  login): horas libres de un barbero reservable que presta el servicio.
- Lectura por contratos: horario, descansos y ausencias (professionals), duración (catalog),
  políticas y zona horaria (businesses); citas activas vacías hasta T6.3.
- Respuesta con hora local (con offset) y UTC; 404 indistinguible; 400 fecha fuera de la ventana.
