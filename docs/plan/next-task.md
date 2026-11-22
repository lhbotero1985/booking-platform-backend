# Siguiente tarea

## Última tarea completada
T3.3 — Caddy, HTTPS e IP real del cliente (2026-11-22), en `feature/t3.3-caddy-https` (commit
y push hechos; PR y merge pendientes de autorización). QA en
https://booking-platform-col.duckdns.org.

**Fase 3 en curso** (desglose aprobado 2026-11-22, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t3.3-caddy-https` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t3.2-qa-compose-stack`.
3. Decisiones para el despliegue desde GitHub (abajo).

## Próxima tarea: T3.4 + T3.5 — Despliegue desde GitHub al servidor QA (pedido por el usuario)

Diseño propuesto, en una sola rama:

- **`release-qa.yml`** (en cada push a `qa`): construye la imagen `linux/amd64`, Trivy
  (HIGH/CRITICAL bloquea), SBOM y procedencia como atestaciones de buildx, publica en
  `ghcr.io/lhbotero1985/booking-platform-backend:qa-<sha>` (privada) y obtiene el digest.
- **Job `deploy`** (GitHub Environment `qa`): SSH al servidor con una **llave dedicada solo para
  CI**, host key fijada en un secreto (sin `ssh-keyscan` en tiempo de ejecución). Envía los
  archivos del stack por stdin y ejecuta `deploy/server/deploy.sh <imagen@digest>`:
  1. `docker login ghcr.io` con el `GITHUB_TOKEN` efímero del job (permiso `packages: read`;
     no queda ninguna credencial de registro en el servidor), `pull`, `logout`.
  2. Backup `pg_dump` antes de migrar.
  3. Migraciones con el servicio `migrate`; `up -d api worker caddy`.
  4. Smoke tests: `/readyz` y versión en `/healthz` = commit desplegado.
  5. Si algo falla: rollback automático a la imagen anterior y el job queda en rojo.
  6. Registro del despliegue (fecha, commit, digest, actor) en el servidor y en el resumen del job.
- **Llave de CI restringida** en `authorized_keys`: `restrict` + `command=` que solo permite
  `deploy <imagen@digest>` (no shell interactiva, sin reenvíos).

Decisiones necesarias del usuario:
1. ¿Despliegue **automático en cada merge a `qa`** (recomendado) o manual (botón "Run workflow")?
   En repos privados del plan Free, los Environments no admiten aprobadores obligatorios.
2. Autorización para crear la llave SSH de CI, instalar su parte pública (restringida) en el
   servidor y guardar la privada y la host key como **secretos del Environment `qa`** en GitHub.
3. GHCR privado: gratis hasta 500 MB en el plan Free; se borrarán imágenes `qa-*` antiguas
   (conservar las últimas 10).
