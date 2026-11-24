# Siguiente tarea

## Última tarea completada
T5.1 — Quitar el rol MANAGER (2026-11-24), en `feature/t5.1-remove-manager-role` (commit y
push; PR y merge pendientes de autorización).

**Fase 5 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T5.2 — Profesionales

- Nuevo módulo `professionals` (o dentro de `businesses`, a decidir con el diseño de
  `domain-model.md` §5): perfil ligado a la cuenta de un miembro del negocio (PROFESSIONAL u
  OWNER que atiende), nombre visible, descripción, activo/inactivo, orden.
- Solo el OWNER crea/edita; un miembro solo tiene un perfil por negocio; retirar la membresía
  desactiva el perfil; otro negocio → 404; auditoría.
