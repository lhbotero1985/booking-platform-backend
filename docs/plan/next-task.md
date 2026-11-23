# Siguiente tarea

## Última tarea completada
T3.6 — Backups diarios de PostgreSQL (2026-11-23), en `feature/t3.6-db-backups` (commit y push;
PR y merge pendientes de autorización). Instalado y probado en QA.

**Fase 3 en curso** (desglose aprobado 2026-11-22, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación → PR `qa` →
`main` → PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente.
PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Bloqueante: push a GHCR denegado en `release-qa`

El primer `release-qa` (commit `40300ea`) falló en el push a GHCR con `denied`. Hipótesis
principal: ya existe un paquete `booking-platform-backend` en la cuenta no vinculado al repo (el
`GITHUB_TOKEN` del repo no tiene acceso). Para diagnosticar se necesita ver los paquetes
(`gh auth refresh -h github.com -s read:packages,delete:packages` o revisar
https://github.com/lhbotero1985?tab=packages).

## Pendiente de autorización

1. PR `feature/t3.4-ghcr-push-fix` → `qa` (imagen de un solo manifiesto) — cada merge despliega.
2. PR `feature/t3.6-db-backups` → `qa`.
3. P38: copia externa de backups QA.

## Próxima tarea: T3.7 — Validación E2E en QA

- Flujos de la Fase 2 por HTTPS público (Mailpit por túnel SSH), primer despliegue automático
  verificado, calibración de Argon2id en el Droplet (P27), manuales de operación.
