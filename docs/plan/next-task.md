# Siguiente tarea

## Última tarea completada
T7.1 — Base de notificaciones (2026-11-24), en `feature/t7.1-notifications-foundation` (commit y
push; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. PR de T7.1 a `qa` y E2E en QA (requiere autorización).
2. T7.2 — Cliente de WhatsApp Cloud API. Requiere del usuario: app de Meta creada con el producto
   WhatsApp, número de prueba, token y su celular registrado como destinatario de prueba. Las
   credenciales van como secretos del servidor de QA, nunca en el chat ni en Git.
