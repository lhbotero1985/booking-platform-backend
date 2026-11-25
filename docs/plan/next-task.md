# Siguiente tarea

## Última tarea completada
T7.8 — Límite de reprogramaciones por cita (2026-11-25), en `feature/t7.8-reschedule-limit`
(commit y push; PR, merge y E2E en QA pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR de T7.8 a `qa`, merge y E2E en QA (requiere autorización).
2. T7.9 — Preferencias de notificación (cliente: WhatsApp y/o correo, al menos uno; recordatorios
   on/off; especialista: correos on/off) y control de costos (P16).
3. Cuando Meta apruebe las plantillas: prueba real de los botones de WhatsApp.
4. Pendiente de autorización: borrar la rama `feature/t7.7-e2e-check` (ya en `qa`, PR #67).
