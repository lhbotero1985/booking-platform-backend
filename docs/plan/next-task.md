# Siguiente tarea

## Última tarea completada
T7.10 — Cierre de la Fase 7 (2026-11-25), en `feature/t7.10-phase7-closure` (commit y push; PR
pendiente de autorización). **El cierre de la fase espera la aprobación del usuario**
(`docs/plan/phase-7-closure.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

1. El usuario revisa y aprueba el cierre de la Fase 7; PR de T7.10 a `qa` (requiere
   autorización).
2. No avanzar a la Fase 8 sin aprobación explícita (ver `roadmap.md`).
3. Cuando Meta apruebe las plantillas: prueba real de mensajes y botones de WhatsApp; plantilla
   nueva con "especialista" (P42).
4. Pendiente de autorización: borrar la rama `feature/t7.9-notification-preferences` (PR #70).
