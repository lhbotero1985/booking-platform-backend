# Siguiente tarea

## Última tarea completada
T3.7 — Validación E2E en QA (2026-11-23), en `feature/t3.7-qa-validation` (commit y push; PR y
merge pendientes de autorización). Al fusionar, `release-qa` despliega `ARGON2_ITERATIONS=2`:
verificar tiempo de login y rehash, y repetir `scripts/e2e-qa.sh`.

**Fase 3 en curso** (desglose aprobado 2026-11-22, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación → PR `qa` →
`main` → PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente.
PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Próxima tarea: T3.8 — PR `qa` → `main` y cierre de la Fase 3

- Checklist de cierre de fase (`docs/plan/phase-3-closure.md`): criterios de T3.1–T3.7,
  riesgos aceptados (P36 sin Cloud Firewall, P37 sin protección de ramas, P38 backups en el mismo
  disco), deuda y preguntas abiertas.
- PR `qa` → `main` (requiere autorización). `main` aún no despliega a ningún lado: PROD es la
  Fase 13.
- Aprobación del cierre de la fase por el usuario.
