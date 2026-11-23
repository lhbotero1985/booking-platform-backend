# Siguiente tarea

## Última tarea completada
T3.7 — Validación E2E en QA (PR #30, `d0f48b1`, desplegado y verificado).

**Fase 3: cierre en curso (T3.8)** — `docs/plan/phase-3-closure.md`.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación → PR `qa` →
`main` (merge commit) + tag `vX.Y.Z` → PROD con el mismo artefacto (Fase 13).

## Pendiente de autorización

1. Aprobación del cierre de la Fase 3.
2. PR `feature/t3.8-phase-3-closure` → `qa` (despliega; se repite el E2E).
3. PR `qa` → `main` con **merge commit** y tag `v0.3.0` (dispara `release-tag.yml`).

## Después: Fase 4

Desglose de la Fase 4 según `roadmap.md`, para aprobación antes de empezar.
