# Siguiente tarea

## Última tarea completada
T5.2 — Profesionales (2026-11-24), en `feature/t5.2-professionals` (commit y push; PR y merge
pendientes de autorización).

**Fase 5 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T5.3 — Horario semanal y descansos

- Franjas de trabajo por día de la semana (hora local del negocio; varias por día, sin solapes),
  definidas por el OWNER.
- Descansos recurrentes (p. ej. almuerzo) dentro de una franja: OWNER o el propio barbero.
- Tablas `professional_working_hours` y `professional_breaks` con FK compuestas
  `(business_id, professional_id)`; reemplazo completo del horario de un día en una transacción.
