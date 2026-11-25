# Siguiente tarea

## Última tarea completada
T7.7 — Acciones desde el correo (2026-11-25): en `qa` (PR #66), E2E QA 214/214.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. El usuario prueba los botones desde Mailpit QA (opcional). PR del ajuste del E2E
   (`feature/t7.7-e2e-check`) pendiente de autorización.
2. T7.8 — Límite de reprogramaciones por cita, configurable por negocio.
3. Cuando Meta apruebe las plantillas: prueba real de los botones de WhatsApp.
