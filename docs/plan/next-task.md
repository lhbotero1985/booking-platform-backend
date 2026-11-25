# Siguiente tarea

## Última tarea completada
T7.9 — Preferencias de notificación y control de costos (2026-11-25), en
`feature/t7.9-notification-preferences` (commit y push; PR, merge y E2E en QA pendientes de
autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR de T7.9 a `qa`, merge y E2E en QA (requiere autorización).
2. T7.10 — E2E en QA con mensajes reales, pruebas de seguridad del webhook y de los enlaces,
   cierre de la Fase 7.
3. Cuando Meta apruebe las plantillas: prueba real de los botones de WhatsApp.
4. Pendiente de autorización: borrar las ramas `feature/deploy-caddy-smoke-retry` (PR #65) y
   `feature/t7.8-reschedule-limit` (PR #68), ya en `qa`.
