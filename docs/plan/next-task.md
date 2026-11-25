# Siguiente tarea

## Última tarea completada
T7.4 — Recordatorios (2026-11-25): PR #59 en `qa`, E2E en QA 200/200.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. Cuando Meta apruebe las plantillas (5 de T7.3 y `recordatorio_cita`): E2E con
   `WHATSAPP_TEST_PHONE`.
2. T7.5 — Webhook de Meta: verificación, firma `X-Hub-Signature-256`, duplicados, estados de
   entrega y respuestas de los botones del recordatorio con códigos de un solo uso. Requiere del
   usuario el **App Secret** de la app de Meta (en el `.env` del servidor) y configurar la URL del
   webhook en Meta.
