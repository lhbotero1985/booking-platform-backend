# Siguiente tarea

## Última tarea completada
T7.5 — Webhook de WhatsApp (2026-11-25), en `feature/t7.5-whatsapp-webhook` (commit y push; PR y
merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR de T7.5 a `qa` y E2E en QA (requiere autorización).
2. Parte B: el usuario registra el webhook en Meta (URL, token de verificación, campo `messages`).
3. Cuando Meta apruebe las plantillas: prueba real — reserva con el celular de prueba, el
   recordatorio llega con botones, "Confirmar cita" confirma y responde.
4. T7.6 — Push (Expo): registro de dispositivos y avisos al barbero.
