# Siguiente tarea

## Última tarea completada
T3.1 — Preparación y endurecimiento del servidor QA (2026-11-22), en
`feature/t3.1-qa-server-bootstrap` (commit y push hechos; PR y merge pendientes de autorización).

**Fase 3 en curso** (desglose aprobado 2026-11-22, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización / decisión

1. PR `feature/t3.1-qa-server-bootstrap` → `qa` y merge si el CI queda en verde.
2. P34 — redimensionar `booking-qa` a 2 GB: `s-1vcpu-2gb-intel`, **US$ 14/mes** en `nyc3`.
3. P35 — DNS gratuito: sslip.io o DuckDNS (necesario en T3.3).
4. P36 — Cloud Firewall de DO (panel del usuario) o solo `ufw`.

## Próxima tarea: T3.2 — Stack QA en Docker Compose

- `deploy/compose/docker-compose.qa.yml`: PostGIS (memoria ajustada al tamaño final del
  Droplet), API y worker desde la imagen (GHCR llega en T3.4; mientras tanto, imagen construida
  en CI o cargada por SSH), Mailpit enlazado a `127.0.0.1`, red interna, límites de memoria,
  `read_only`, `no-new-privileges`, ningún puerto publicado salvo Caddy (T3.3).
- Roles de BD: `app_migrator` (dueño del esquema, solo migraciones) y `app_runtime` (DML, sin
  DDL); `DATABASE_MIGRATOR_URL` y `DATABASE_URL` separados.
- Secretos de QA generados en el servidor (`/opt/booking/qa/.env`, 0600, `deploy`), nunca en Git.
- Pruebas: stack sano, `/readyz` OK, `app_runtime` no puede hacer DDL, puertos internos cerrados.
