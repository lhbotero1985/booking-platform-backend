# Despliegue QA

Estado: **En construcción (Fase 3).** Servidor (T3.1) y stack (T3.2) operativos; HTTPS con
Caddy (T3.3) y despliegue automático (T3.5) pendientes.

## 1. Datos del ambiente

| Dato | Valor |
|---|---|
| Servidor | `booking-qa` · `174.138.92.121` · ver `server-bootstrap.md` |
| Dominio | `booking-platform-col.duckdns.org` (DuckDNS, gratuito; P35) |
| Directorio | `/opt/booking/qa/` (`docker-compose.yml`, `qa/initdb/`, `.env`) |
| Proyecto Compose | `booking-qa` |
| Secretos | `/opt/booking/qa/.env` (0600, `deploy`), generados en el servidor por `deploy/server/qa-env.sh`; nunca salen del servidor |

## 2. Stack (`deploy/compose/docker-compose.qa.yml`)

| Servicio | Límite de memoria | Exposición | Notas |
|---|---|---|---|
| `postgres` (PostGIS 17-3.5, fijado por digest) | 320 MiB | Ninguna (red interna) | `shared_buffers=64MB`, `max_connections=40`, SCRAM; `read_only` con tmpfs |
| `api` | 256 MiB (`GOMEMLIMIT=180MiB`) | `127.0.0.1:8080` (solo el host) | `ARGON2_MAX_CONCURRENT=1`; rol `app_runtime` |
| `worker` | 160 MiB (`GOMEMLIMIT=110MiB`) | Ninguna | Rol `app_runtime` |
| `mailpit` | 64 MiB | `127.0.0.1:8025` (solo túnel SSH) | Ningún correo real sale de QA |
| `migrate` (perfil `tools`) | 128 MiB | Ninguna | Único servicio con la contraseña de `app_migrator` |

Todos los contenedores: `read_only`, `cap_drop: ALL` (salvo PostgreSQL), `no-new-privileges`.
Docker evita ufw: **ningún puerto se publica en interfaces públicas**; solo Caddy (T3.3)
publicará 80/443.

### Roles de base de datos (`deploy/compose/qa/initdb/10-app-roles.sh`)

Se crean una sola vez, al inicializar el volumen de datos:

| Rol | Permisos |
|---|---|
| `booking_admin` | Superusuario (solo inicialización y emergencias) |
| `app_migrator` | Dueño de la base y del esquema `public`; DDL (migraciones) |
| `app_runtime` | CONNECT; SELECT/INSERT/UPDATE/DELETE en tablas, uso de secuencias y funciones creadas por `app_migrator` (privilegios por defecto); sin DDL, sin TRUNCATE, sin acceso a otras bases |

Las extensiones (`postgis`, `btree_gist`, `citext`) las crea el superusuario en ese script
(PostGIS no es una extensión *trusted*); la migración 00001 queda como no-op.

## 3. Primer despliegue (realizado 2026-11-22)

```bash
# Secretos (idempotente: no sobrescribe un .env existente)
ssh deploy@HOST 'bash -s' < deploy/server/qa-env.sh
# Archivos del stack (git archive conserva los permisos de ejecución)
git archive HEAD deploy/compose/docker-compose.qa.yml deploy/compose/qa \
  | ssh deploy@HOST 'cd /opt/booking/qa && tar -x --strip-components=2 -f - && mv -f docker-compose.qa.yml docker-compose.yml'
# Imagen (hasta T3.4 se transfiere por SSH; después se descarga de GHCR)
docker save booking-platform-backend:qa-<sha> | gzip | ssh deploy@HOST 'gunzip | docker load'
ssh deploy@HOST "sed -i 's#^API_IMAGE=.*#API_IMAGE=booking-platform-backend:qa-<sha>#' /opt/booking/qa/.env"
# Arranque
ssh deploy@HOST 'cd /opt/booking/qa && docker compose up -d postgres mailpit \
  && docker compose run --rm migrate up && docker compose up -d api worker'
```

## 4. Verificación y acceso

```bash
ssh deploy@HOST 'curl -s 127.0.0.1:8080/readyz; curl -s 127.0.0.1:8080/healthz'
# API y Mailpit desde tu equipo (túnel SSH; nada se expone a Internet)
ssh -N -L 18080:127.0.0.1:8080 -L 18025:127.0.0.1:8025 deploy@HOST
#   API:     http://127.0.0.1:18080
#   Mailpit: http://127.0.0.1:18025
```

## 5. Pendiente

- HTTPS con Caddy, IP real del cliente (T3.3).
- Imagen desde GHCR (T3.4) y despliegue automático con rollback (T3.5).
- Backups (T3.6) y registro de despliegues (fecha, commit, digest, actor, smoke tests).
