# Siguiente tarea

## Última tarea completada
T7.5 — Webhook de WhatsApp (2026-11-25), validada en QA (PR #60, `8363924`, E2E 203/203).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. Parte B: el usuario registra el webhook en Meta (URL, token de verificación, campo `messages`).
2. Cuando Meta apruebe las plantillas: prueba real — reserva con el celular de prueba, el
   recordatorio llega con botones, "Confirmar cita" confirma y responde.
3. T7.6 — Push (Expo): registro de dispositivos y avisos al barbero.
