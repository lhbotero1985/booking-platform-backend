# Siguiente tarea

## Última tarea completada
T6.8 — Cierre de la Fase 6 (2026-11-24), en `feature/t6.8-phase6-closure` (commit y push; PR y
merge pendientes de autorización).

**Fase 6 aprobada por el usuario (2026-11-24):** ver `docs/plan/phase-6-closure.md`.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

Fase 7 — WhatsApp y push: proponer el desglose en tareas para aprobación del usuario, empezando
por P16/P17: el usuario no tiene cuenta de Meta Business (trámite a iniciar; mientras tanto se puede
desarrollar con el número de prueba de Meta). P40 ya decidido: WhatsApp con el enlace de la app al
agendar manualmente. P13 (email de PROD) sigue pendiente.
