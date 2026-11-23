# Siguiente tarea

## Última tarea completada
T3.4 + T3.5 — Despliegue desde GitHub a QA (2026-11-23), en `feature/t3.4-deploy-from-github`
(commit y push hechos; PR y merge pendientes de autorización). Al fusionar, `release-qa`
desplegará automáticamente: esa ejecución es la validación final.

**Fase 3 en curso** (desglose aprobado 2026-11-22, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación → PR `qa` →
`main` → PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente.
PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t3.4-deploy-from-github` → `qa` y merge si el CI queda en verde (dispara el
   primer despliegue automático).
2. P37: protección de ramas en el plan Free.

## Próxima tarea: T3.6 — Backups diarios de PostgreSQL

- Timer systemd (o cron) en el servidor: `pg_dump -Fc` diario, retención (p. ej. 7 diarios + 4
  semanales), en `/opt/booking/qa/backups/` (gratuito, mismo disco; P34/decisión sin costo).
- Prueba de restauración real en una base temporal y verificación de conteos.
- Documentar en `docs/operations/backup-restore.md`.
- Riesgo aceptado: backups en el mismo Droplet (sin copia externa) mientras no se autorice
  Spaces u otra ubicación.
