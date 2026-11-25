# Siguiente tarea

## Última tarea completada
T7.3 — Mensajes de la cita (2026-11-25): PR #58 en `qa`, E2E en QA 198/198.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. Cuando Meta apruebe las 5 plantillas: E2E en QA con `WHATSAPP_TEST_PHONE` (el usuario recibe
   "cita_agendada" con datos reales).
2. T7.4 — Recordatorios: proponer el texto de la plantilla de recordatorio con botones Confirmar /
   Cancelar (que el usuario crea en Meta) y el job programado según el negocio.
