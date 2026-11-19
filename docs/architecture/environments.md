# Ambientes DEV, QA y PROD

Estado: **Propuesta — Fase 0.**

| Aspecto | DEV | QA | PROD |
|---|---|---|---|
| Creación | Fase 1 | **Fase 3** (con autorización de costos) | **Fase 13** |
| Ubicación | Máquina local (Docker Desktop) | Droplet DigitalOcean exclusivo | Droplet DigitalOcean exclusivo |
| Orquestación | Docker Compose | Docker Compose | Docker Compose |
| API / Worker | Contenedores o `go run` | Contenedores (imagen GHCR por digest) | Misma imagen validada en QA |
| PostgreSQL | Contenedor `postgis/postgis` | Contenedor en el Droplet + volumen DO | Contenedor + volumen (o DO Managed PG ❓) |
| TLS | No (localhost) | Caddy + Let's Encrypt | Caddy + Let's Encrypt |
| Dominio | `localhost` | `api.qa.<dominio>` ❓ | `api.<dominio>` ❓ |
| Datos | Sintéticos (seed) | Sintéticos | Reales |
| WhatsApp | Deshabilitado (adaptador fake) | Número de prueba de Meta + lista blanca de destinatarios | Número productivo |
| Email | Mailpit (UI 127.0.0.1:8025) | **Mailpit** en la red interna; UI solo por túnel SSH; ningún correo real | Proveedor SMTP con TLS (P13) |
| Push | Fake / Expo dev | Expo (proyecto QA) | Expo (proyecto PROD) |
| Secretos | `.env` local (ignorado) | GitHub Environment `qa` + archivo en servidor (0600) | GitHub Environment `prod` + archivo en servidor (0600) |
| Backups | No | Diario, retención 7 días | Diario + WAL ❓, retención 30 días, prueba de restauración mensual |
| Logs | Consola | JSON a disco con rotación | JSON con rotación + alertas |
| Despliegue | Manual | GitHub Actions con aprobación | GitHub Actions con aprobación explícita |

## Aislamiento QA ↔ PROD

No comparten: Droplet, base de datos, volúmenes, credenciales, secretos, sesiones, backups,
buckets/archivos privados, webhooks (URL y verify token distintos), proyectos de Expo, App
Secret de Meta (app de Meta distinta o al menos número y token distintos), llaves SSH.

## Configuración

Todo por variables de entorno (12-factor), validadas al iniciar. `.env.example` (Fase 1) lista
cada variable sin valores reales. Variables previstas: `APP_ENV`, `HTTP_ADDR`, `DATABASE_URL`,
`DATABASE_MIGRATOR_URL`, `LOG_LEVEL`, `CORS_ALLOWED_ORIGINS`, `SESSION_*`, `ARGON2_*`,
`WHATSAPP_ENABLED`, `WHATSAPP_PHONE_NUMBER_ID`, `WHATSAPP_ACCESS_TOKEN`, `WHATSAPP_APP_SECRET`,
`WHATSAPP_VERIFY_TOKEN`, `EXPO_ACCESS_TOKEN`, `EMAIL_*`.
