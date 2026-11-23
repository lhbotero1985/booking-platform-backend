# Despliegue QA

Estado: **En construcción (Fase 3).** Servidor (T3.1), stack (T3.2) y HTTPS con Caddy (T3.3)
operativos en **https://booking-platform-col.duckdns.org**. Despliegue automático desde GitHub en
cada merge a `qa` (T3.4/T3.5).

## 1. Datos del ambiente

| Dato | Valor |
|---|---|
| Servidor | `booking-qa` · `174.138.92.121` · ver `server-bootstrap.md` |
| Dominio | `booking-platform-col.duckdns.org` (DuckDNS, gratuito; P35) |
| Directorio | `/opt/booking/qa/` (`docker-compose.yml`, `qa/`, `.env`, `backups/`, `rollback/`, `deployments.log`) |
| Proyecto Compose | `booking-qa` |
| Secretos | `/opt/booking/qa/.env` (0600, `deploy`), generados en el servidor por `deploy/server/qa-env.sh`; nunca salen del servidor |

## 2. Stack (`deploy/compose/docker-compose.qa.yml`)

| Servicio | Límite de memoria | Exposición | Notas |
|---|---|---|---|
| `postgres` (PostGIS 17-3.5, fijado por digest) | 320 MiB | Ninguna (red interna) | `shared_buffers=64MB`, `max_connections=40`, SCRAM; `read_only` con tmpfs |
| `api` | 256 MiB (`GOMEMLIMIT=180MiB`) | `127.0.0.1:8080` (solo el host) | `ARGON2_MAX_CONCURRENT=1`; rol `app_runtime` |
| `worker` | 160 MiB (`GOMEMLIMIT=110MiB`) | Ninguna | Rol `app_runtime` |
| `mailpit` | 64 MiB | `127.0.0.1:8025` (solo túnel SSH) | Ningún correo real sale de QA |
| `caddy` (2.11.9, fijado por digest) | 96 MiB | **80, 443/tcp, 443/udp** (único servicio público) | HTTPS automático (Let's Encrypt), HSTS 1 año, HTTP/3, cuerpo ≤ 2 MB, reintentos 10 s hacia la API; IP fija `172.30.0.10` |
| `migrate` (perfil `tools`) | 128 MiB | Ninguna | Único servicio con la contraseña de `app_migrator` |

Todos los contenedores: `read_only`, `cap_drop: ALL` (salvo PostgreSQL; Caddy solo
`NET_BIND_SERVICE`), `no-new-privileges`. Docker evita ufw: solo Caddy publica puertos en
interfaces públicas; el resto escucha en `127.0.0.1` o en la red interna `172.30.0.0/24`.

### IP real del cliente

La API cree `X-Forwarded-For` **solo** si la conexión viene de `TRUSTED_PROXIES=172.30.0.10`
(Caddy). Caddy sobrescribe la cabecera con la IP real (ignora la que envíe el cliente). Así, rate
limiting, logs y `audit_log` ven la IP pública del cliente; un `X-Forwarded-For` falso no tiene
efecto (verificado en QA).

### Certificados

Caddy guarda certificados y cuenta ACME en el volumen `caddy_data`: **no borrarlo** (Let's
Encrypt limita emisiones repetidas). La renovación es automática (≈30 días antes de vencer).

### Roles de base de datos (`deploy/compose/qa/initdb/10-app-roles.sh`)

Se crean una sola vez, al inicializar el volumen de datos:

| Rol | Permisos |
|---|---|
| `booking_admin` | Superusuario (solo inicialización y emergencias) |
| `app_migrator` | Dueño de la base y del esquema `public`; DDL (migraciones) |
| `app_runtime` | CONNECT; SELECT/INSERT/UPDATE/DELETE en tablas, uso de secuencias y funciones creadas por `app_migrator` (privilegios por defecto); sin DDL, sin TRUNCATE, sin acceso a otras bases |

Las extensiones (`postgis`, `btree_gist`, `citext`) las crea el superusuario en ese script
(PostGIS no es una extensión *trusted*); la migración 00001 queda como no-op.

## 3. Despliegue automático desde GitHub (T3.4/T3.5)

Cada push a `qa` (merge de un PR) ejecuta `.github/workflows/release-qa.yml`:

| Job | Qué hace |
|---|---|
| `image` | `docker build` linux/amd64 (versión = commit corto) → **Trivy** (HIGH/CRITICAL con fix bloquea) → **SBOM** CycloneDX (artefacto del workflow, 90 días) → push de **la misma imagen escaneada** a `ghcr.io/lhbotero1985/booking-platform-backend:qa-<sha>` (privada) → digest |
| `deploy` | Environment `qa` (solo la rama `qa` puede desplegar). SSH con la llave de CI y host key fijada → `deploy <imagen@digest> <versión> <actor>` en el servidor → smoke test público por HTTPS (`/healthz` = versión, `/readyz`) → registro en el resumen del job |
| `prune` | Borra de GHCR las versiones `qa-*` más allá de las 10 más recientes y las versiones sin tag |

### Script del servidor: `/usr/local/bin/booking-deploy` (fuente `deploy/server/deploy.sh`)

1. Valida la entrada: solo `deploy`, imagen **de este repositorio en GHCR fijada por digest**,
   versión hexadecimal, actor válido; un despliegue a la vez (`flock`).
2. `docker login` con el `GITHUB_TOKEN` efímero del job (llega por stdin, nunca en la línea de
   comandos), `pull`, `logout`; la configuración temporal de Docker se borra.
3. Extrae el stack (`docker-compose.yml`, `qa/Caddyfile`, `qa/initdb/`) **desde la imagen**
   (`/deploy/qa/`) y lo valida con `docker compose config`. CI nunca envía archivos.
4. Backup `pg_dump -Fc` en `backups/predeploy-<fecha>-<versión>.dump` (0600; se conservan 5).
5. Guarda el stack actual en `rollback/`, instala el nuevo, `migrate up`, `up -d`; reinicia
   Caddy si cambió el Caddyfile.
6. Smoke tests: `/readyz` y versión en `/healthz` (60 s), `/readyz` a través de Caddy (TLS) y
   worker en ejecución.
7. Éxito: `API_IMAGE` en `.env` y línea en `deployments.log`; borra imágenes viejas (conserva la
   actual y la anterior). Fallo: **rollback automático** (stack + imagen anterior, smoke test),
   `rolled-back` en el log y el job queda en rojo. Las migraciones no se revierten
   (expand → migrate → contract).

### Llave de CI

- ed25519 exclusiva; la privada solo existe como secreto `QA_SSH_KEY` del Environment `qa`
  (junto con `QA_SSH_KNOWN_HOSTS` y la variable `QA_SSH_HOST`).
- En `~deploy/.ssh/authorized_keys`: `restrict,command="/usr/local/bin/booking-deploy" … github-actions-qa-deploy`.
  Sin shell, sin reenvío de puertos/agente, sin PTY (verificado).
- Aunque la llave se filtre, solo permite desplegar imágenes ya publicadas en el GHCR de este
  repositorio (publicar exige permisos de escritura en el repo).
- Rotación: generar una llave nueva, reemplazar la línea en `authorized_keys` y el secreto.

### Actualizar el script del servidor

El script es de root y la llave de CI no puede modificarlo. Tras cambiar `deploy/server/deploy.sh`:

```bash
ssh deploy@HOST 'sudo install -m 0755 -o root -g root /dev/stdin /usr/local/bin/booking-deploy' \
  < deploy/server/deploy.sh
```

### Despliegue manual (emergencia o re-despliegue de un digest anterior)

```bash
# Token con read:packages (p. ej. gh auth token con ese scope); llega por stdin
gh auth token | ssh deploy@HOST 'booking-deploy deploy ghcr.io/lhbotero1985/booking-platform-backend@sha256:<digest> <versión> <actor>'
```

## 4. Primer despliegue (manual, realizado 2026-11-22)

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

## 5. Verificación y acceso

```bash
ssh deploy@HOST 'curl -s 127.0.0.1:8080/readyz; curl -s 127.0.0.1:8080/healthz'
# API y Mailpit desde tu equipo (túnel SSH; nada se expone a Internet)
ssh -N -L 18080:127.0.0.1:8080 -L 18025:127.0.0.1:8025 deploy@HOST
#   API:     http://127.0.0.1:18080
#   Mailpit: http://127.0.0.1:18025
```

Registro de despliegues: `cat /opt/booking/qa/deployments.log` (fecha, resultado, versión,
imagen, actor) y el resumen de cada ejecución de `release-qa` en GitHub Actions.

## 6. Backups

Diarios con `booking-backup.timer` (T3.6); ver `backup-restore.md`.
