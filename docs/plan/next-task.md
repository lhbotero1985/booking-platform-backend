# Siguiente tarea

## Última tarea completada
T5.3 — Horario semanal y descansos (2026-11-24), en `feature/t5.3-weekly-schedule` (commit y
push; PR y merge pendientes de autorización).

**Fase 5 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T5.4 — Ausencias e incapacidades

- Ausencias de un profesional con inicio y fin (`tstzrange` en UTC; vacaciones, incapacidad,
  diligencia) y motivo opcional.
- Permisos (decisión 2026-11-24): OWNER para cualquier barbero; el barbero registra, modifica y
  elimina las suyas; los demás del equipo las ven.
- Sin solapes por profesional: `EXCLUDE USING gist (professional_id WITH =, period WITH &&)`
  (extensión `btree_gist`), FK compuesta `(business_id, professional_id)`.
- Las citas manuales del barbero quedan para la Fase 6 (motor de reservas).
