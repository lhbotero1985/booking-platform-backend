# Siguiente tarea

## Última tarea completada
T7.3 — Mensajes de la cita (2026-11-25), en `feature/t7.3-appointment-templates` (commit y push;
PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. Cuando Meta apruebe las 5 plantillas: PR de T7.3 a `qa` y E2E en QA con `WHATSAPP_TEST_PHONE`
   (el usuario recibe "cita_agendada" con datos reales).
2. T7.4 — Recordatorios: proponer el texto de la plantilla de recordatorio con botones Confirmar /
   Cancelar (que el usuario crea en Meta) y el job programado según el negocio.
