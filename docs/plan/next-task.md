# Siguiente tarea

## Última tarea completada
T6.5 — Agenda y mis citas (2026-11-24), en `feature/t6.5-agenda` (commit y push; PR y merge
pendientes de autorización).

**Fase 6 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T6.6 — Cambios de estado de la cita

- Confirmar, cancelar y reprogramar (con los límites de horas del negocio para el cliente),
  iniciar, completar, "no asistió"; historial en `appointment_events`.
- Cliente reprograma → sigue `CONFIRMED`; negocio reprograma → `PENDING_CONFIRMATION` (P7).
- Transición inválida → 409; confirmaciones repetidas idempotentes; concurrencia con `version`.
