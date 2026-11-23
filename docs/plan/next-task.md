# Siguiente tarea

## Última tarea completada
T3.6 — Backups diarios (PR #28, `9b169a9`) y despliegue automático a QA operativo (PR #27, #29).
QA corre `9b169a9` desplegado por `release-qa`.

**Fase 3 en curso** (desglose aprobado 2026-11-22, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación → PR `qa` →
`main` → PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente.
PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Próxima tarea: T3.7 — Validación E2E en QA (rama `feature/t3.7-qa-validation`)

- Flujos de la Fase 2 por HTTPS público (registro, verificación por Mailpit vía túnel SSH, login,
  `/me`, membresías, logout, reset y cambio de contraseña, rate limiting, auditoría con IP real).
- Calibración de Argon2id en el Droplet (P27): objetivo ≈100–250 ms por hash con 1 GB de RAM.
- Manuales de operación (`docs/operations/`): revisión final de QA.
