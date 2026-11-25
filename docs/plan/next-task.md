# Siguiente tarea

## Última tarea completada
T7.8 — Límite de reprogramaciones por cita (2026-11-25): en `qa` (PR #68), E2E QA 218/218.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR del registro de la validación de T7.8 (`feature/t7.8-qa-validation`, solo docs) pendiente de
   autorización.
2. T7.9 — Preferencias de notificación (cliente: WhatsApp y/o correo, al menos uno; recordatorios
   on/off; especialista: correos on/off) y control de costos (P16).
3. Cuando Meta apruebe las plantillas: prueba real de los botones de WhatsApp.
