# Siguiente tarea

## Última tarea completada
T1.2 — Plataforma HTTP base (2026-11-18).

## Acción pendiente del usuario: primer push a GitHub

El push está autorizado, pero Claude Code no pudo completarlo (requiere iniciar sesión en
GitHub de forma interactiva). Ejecutar en una terminal:

```powershell
cd C:\Users\PC-1\Desktop\booking-platform\booking-platform-backend
git push -u origin main
```

Git Credential Manager abrirá el navegador para autenticarse con GitHub la primera vez.
Después: verificar que el workflow `security` (gitleaks) pase en la pestaña Actions.

## Próxima tarea: T1.3 — Docker y Docker Compose (DEV)

- **Objetivo:** imagen mínima y segura del backend + entorno DEV reproducible con PostgreSQL/PostGIS.
- **Archivos:**
  - `deploy/docker/Dockerfile` — multi-stage, `CGO_ENABLED=0`, `-trimpath`, versión por
    build-args, imagen final `distroless/static` no-root, `time/tzdata` embebido.
  - `deploy/compose/docker-compose.dev.yml` — `postgres` (postgis, volumen nombrado, healthcheck,
    puerto solo en 127.0.0.1), `api` (read_only, no-new-privileges, límites de memoria).
  - `.env.example` — todas las variables sin valores reales.
  - `.dockerignore`.
  - `Taskfile.yml` — `task up`, `task down`, `task logs`, `task docker:build`.
  - `.github/dependabot.yml` — ecosistema `docker`.
- **Pruebas:** build de la imagen; `task up` → `/healthz` 200 desde el host; SIGTERM al
  contenedor → log "api stopped cleanly"; contenedor corre como no-root; Trivy `config` e
  `image` sin HIGH/CRITICAL con fix disponible.
- **Comandos de validación:** `task check`, `task docker:build`, `task up`, `docker compose ps`,
  Trivy vía Docker.
- **Criterios de aceptación:** los anteriores, más que ningún secreto real quede en archivos
  versionados.
- **Bloqueos:** ninguno (Docker Desktop activo).
