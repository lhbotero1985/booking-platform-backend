# Siguiente tarea

## Última tarea completada
T7.6 — Correos de citas (2026-11-25), en `feature/t7.6-appointment-emails` (commit y push; PR y
merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR de T7.6 a `qa`, merge y E2E en QA (requiere autorización); el usuario aprueba el diseño
   viendo el correo en Mailpit.
2. T7.7 — Acciones desde el correo en dos toques (confirmar, cancelar; reagendar con servicios
   para el frontend).
3. Cuando Meta apruebe las plantillas: prueba real de los botones de WhatsApp.
