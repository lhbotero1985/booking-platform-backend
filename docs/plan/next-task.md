# Siguiente tarea

## Última tarea completada
T7.6 — Correos de citas (2026-11-25), validada en QA (PR #61, `3cf9b43`, E2E 209/209).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. El usuario aprueba el diseño del correo en Mailpit QA.
2. T7.7 — Acciones desde el correo en dos toques (confirmar, cancelar; reagendar con servicios
   para el frontend).
3. Cuando Meta apruebe las plantillas: prueba real de los botones de WhatsApp.
