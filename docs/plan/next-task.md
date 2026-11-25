# Siguiente tarea

## Última tarea completada
T7.2 — Cliente de WhatsApp Cloud API (2026-11-25), en `feature/t7.2-whatsapp-cloud-api` (commit y
push; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR de T7.2 a `qa` y E2E en QA con `WHATSAPP_TEST_PHONE` (el usuario recibe un WhatsApp real).
2. T7.3 — Mensajes al cliente: proponer al usuario los textos de las plantillas (cita creada,
   cancelada por el negocio, reprogramada, reasignada; enlace de la app en la cita manual, P40),
   que el usuario crea y envía a aprobación en Meta.
