# Siguiente tarea

## Última tarea completada
T7.4 — Recordatorios (2026-11-25), en `feature/t7.4-reminders` (commit y push; PR y merge
pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR de T7.4 a `qa` y E2E en QA (requiere autorización).
2. Cuando Meta apruebe las plantillas (5 de T7.3 y `recordatorio_cita`): E2E con
   `WHATSAPP_TEST_PHONE`.
3. T7.5 — Webhook de Meta: verificación, firma `X-Hub-Signature-256`, duplicados, estados de
   entrega y respuestas de los botones del recordatorio con códigos de un solo uso. Requiere del
   usuario el **App Secret** de la app de Meta (en el `.env` del servidor) y configurar la URL del
   webhook en Meta.
