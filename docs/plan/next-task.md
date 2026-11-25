# Siguiente tarea

## Última tarea completada
T7.7 — Acciones desde el correo (2026-11-25), en `feature/t7.7-email-actions` (commit y push; PR y
merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR de T7.7 a `qa`, merge y E2E en QA (requiere autorización); el usuario prueba los botones
   desde Mailpit.
2. T7.8 — Límite de reprogramaciones por cita, configurable por negocio.
3. Cuando Meta apruebe las plantillas: prueba real de los botones de WhatsApp.
