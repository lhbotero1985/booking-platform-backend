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

## 3B. Portales web del frontend (Booka; fase F2 del frontend)

Los portales `business-web` y `admin-web` del repositorio `booking-platform-frontend` corren en
este mismo Droplet (2 GB desde 2026-11-25), en proyectos Compose propios
(`deploy/compose/qa/web/docker-compose.yml`, que viaja en la imagen del backend como el resto del
stack) unidos a la red `booking-qa_default` con los alias `business-web` y `admin-web`.

| Host (Caddy) | `/api/*` | Resto |
|---|---|---|
| `booka-app-qa.duckdns.org` (`APP_DOMAIN`) | `api:8080` | `business-web:3000` |
| `booka-admin-qa.duckdns.org` (`ADMIN_DOMAIN`) | `api:8080` | `admin-web:3000` |

Mismo origen para `/api/*` (ADR-F003 del frontend): la cookie de sesión queda separada por portal
y no hace falta CORS; el `Origin` de cada portal debe estar en `CORS_ALLOWED_ORIGINS` para el
control CSRF. Mientras un portal no esté desplegado su host responde 502 sin afectar a la API.

**Despliegue:** el workflow del frontend publica cada imagen en
`ghcr.io/lhbotero1985/booking-platform-frontend/<portal>` y ejecuta por SSH, con **su propia llave**
restringida a `/usr/local/bin/booka-web-deploy` (fuente `deploy/server/web-deploy.sh`):
`deploy-web <portal> <imagen@digest> <versión> <actor>` (token del registro por stdin). El script
solo acepta imágenes de ese repositorio fijadas por digest, levanta únicamente ese portal, exige
contenedor `healthy` y `/healthz` por HTTPS a través de Caddy, y si falla vuelve a la imagen
anterior (o detiene el portal en su primer despliegue). Estado en `web-images.env`; historial en
`deployments.log`. Nunca toca la API ni la base de datos.

**Pasos únicos del operador** (requieren el Droplet encendido y los registros DuckDNS creados):

```bash
# 1. Variables de los portales en el .env existente (qa-env.sh solo las crea en instalaciones nuevas)
ssh deploy@HOST 'cd /opt/booking/qa && cp -p .env .env.bak &&   printf "APP_DOMAIN=booka-app-qa.duckdns.org
ADMIN_DOMAIN=booka-admin-qa.duckdns.org
" >> .env &&   sed -i "s#^CORS_ALLOWED_ORIGINS=.*#CORS_ALLOWED_ORIGINS=https://booking-platform-col.duckdns.org,https://booka-app-qa.duckdns.org,https://booka-admin-qa.duckdns.org#" .env'
# 2. Script del servidor (root, la llave de CI no puede cambiarlo)
ssh deploy@HOST 'sudo install -m 0755 -o root -g root /dev/stdin /usr/local/bin/booka-web-deploy'   < deploy/server/web-deploy.sh
# 3. Llave de CI del frontend (ed25519 nueva) en ~deploy/.ssh/authorized_keys:
#    restrict,command="/usr/local/bin/booka-web-deploy" ssh-ed25519 AAAA… github-actions-frontend-qa
#    La privada va como secreto QA_WEB_SSH_KEY del Environment `qa` del repositorio frontend,
#    junto con QA_SSH_KNOWN_HOSTS y la variable QA_SSH_HOST.
# 4. Desplegar este cambio del backend (Caddy se reinicia solo si cambió el Caddyfile) y
#    verificar: curl -I https://booka-app-qa.duckdns.org/  → 502 hasta el primer despliegue web.
```

`PUBLIC_APP_URL=https://booka-app-qa.duckdns.org` desde el 2026-11-26 (P-6): los enlaces de los
correos de cuenta abren `/verify-email` y `/reset-password` de business-web. Aplicado con copia
`.env.bak-p6` y `docker compose up -d api worker`.

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

### Validación E2E (`scripts/e2e-qa.sh`, T3.7)

52 verificaciones de los flujos de la Fase 2 contra el entorno desplegado (usuario sintético
`qa-e2e-…@example.test`): plataforma (sondas, HSTS, sin `Server`, `X-Request-ID`, RFC 9457),
registro y verificación por correo, anti-enumeración, login/`/me`/membresías, cambio y
recuperación de contraseña con revocación de sesiones, logout/logout-all y rate limiting. Respeta
`Retry-After` (las rutas de autenticación permiten 10/min por IP); tarda ≈2 min.

```bash
ssh -f -N -M -S ~/.ssh/bp-tun -L 18025:127.0.0.1:8025 deploy@HOST          # Mailpit
scripts/e2e-qa.sh https://booking-platform-col.duckdns.org http://127.0.0.1:18025
# Sin bash/jq locales (Windows): en un contenedor
docker run --rm -v "$PWD/scripts:/s:ro" alpine:3.22 sh -c \
  "apk add -q bash curl jq && bash /s/e2e-qa.sh https://booking-platform-col.duckdns.org http://host.docker.internal:18025"
ssh -S ~/.ssh/bp-tun -O exit deploy@HOST                                      # cerrar túnel
```

Con `ADMIN_EMAIL`/`ADMIN_PASSWORD` (cuenta `PLATFORM_ADMIN` de QA) el script también prueba el
alta de negocios; cada corrida deja un negocio `QA E2E <timestamp>` en estado `SUSPENDED`. Cada
corrida también deja un negocio autoservicio `QA E2E propia <timestamp>` cuya demo vence sola a
los 7 días.

### Vencimiento de demos (T4.3b)

El worker ejecuta cada hora (y al arrancar) el job `subscriptions.expire_trials` (cola
`maintenance`, único por hora): las suscripciones `TRIALING` vencidas pasan a `EXPIRED` y se
auditan (`subscription.expired`, `source=job`). Revisar con
`docker compose logs worker | grep "demos expired"`.

Después, comprobar en `audit_log` que las acciones quedaron con la IP pública del cliente.

### Administradores de la plataforma (T4.1)

`PLATFORM_ADMIN` no se otorga por HTTP. Con acceso al servidor (la cuenta debe existir y estar
verificada):

```bash
ssh deploy@HOST 'cd /opt/booking/qa && docker compose exec api /usr/local/bin/api admin grant-platform-admin EMAIL'
ssh deploy@HOST 'cd /opt/booking/qa && docker compose exec api /usr/local/bin/api admin revoke-platform-admin EMAIL'
```

Queda en `audit_log` (`admin.platform_admin_granted|revoked`, `source=cli`). El cambio aplica en la
siguiente petición del usuario.

### WhatsApp (Meta Cloud API, T7.2)

Las credenciales viven solo en `/opt/booking/qa/.env` (permisos `600`, dueño `deploy`), nunca en
Git ni en GitHub. Se agregan o rotan editando el archivo en el servidor (con `nano`, para que el
token no quede en el historial de la consola):

```
WHATSAPP_ACCESS_TOKEN=<token permanente del usuario del sistema de Meta>
WHATSAPP_PHONE_NUMBER_ID=<identificador del número, no el número>
WHATSAPP_BUSINESS_ACCOUNT_ID=<identificador de la cuenta de WhatsApp Business>
WHATSAPP_ALLOWLIST=+57XXXXXXXXXX[,+57…]   # obligatorio fuera de PROD; máx. 5 con el número de prueba
```

- Verificar sin mostrar valores: `grep '^WHATSAPP_' /opt/booking/qa/.env | cut -d= -f1`.
- Aplicar sin redesplegar: `cd /opt/booking/qa && docker compose --env-file .env -f <stack>/docker-compose.yml up -d worker`
  (o esperar al siguiente despliegue, que recrea los contenedores con el `.env`).
- El log del worker dice al arrancar `whatsapp sender` con `provider=meta` o `simulated`.
- Token comprometido: revocarlo en el usuario del sistema de Meta, generar otro y reemplazarlo.
- **Nunca enviar capturas del `.env`** (contiene también las contraseñas de la base).
- Webhook (T7.5): `WHATSAPP_APP_SECRET` (Configuración de la app → Básica → Clave secreta) con
  `nano`, y el token de verificación generado en el servidor:
  `echo "WHATSAPP_WEBHOOK_VERIFY_TOKEN=$(openssl rand -hex 24)" >> /opt/booking/qa/.env`.
  Registro en Meta (caso de uso WhatsApp → Configuración → Webhook): URL
  `https://booking-platform-col.duckdns.org/webhooks/whatsapp`, el token de verificación
  (`grep '^WHATSAPP_WEBHOOK_VERIFY_TOKEN=' /opt/booking/qa/.env | cut -d= -f2`), "Verificar y
  guardar" y suscribirse al campo `messages`. Solo después de desplegar T7.5.
- Enlace de la app (`GET /app`, botón de `cita_agendada_negocio`): sin `APP_DOWNLOAD_URL` muestra
  "muy pronto"; cuando la app esté en las tiendas, agregar `APP_DOWNLOAD_URL=https://…` al `.env`
  (y al compose de la API) y redesplegar.
- E2E con un envío real: `WHATSAPP_TEST_PHONE=<número de la lista blanca> scripts/e2e-qa.sh …`
  (recibe exactamente un mensaje; el número no se imprime).

### Argon2id (P27)

QA: `ARGON2_MEMORY_KIB=65536` (64 MiB), `ARGON2_ITERATIONS=2`, `ARGON2_PARALLELISM=2`,
`ARGON2_MAX_CONCURRENT=1`: ≈220–260 ms por hash en el vCPU compartido (t=3 ≈ 340 ms). Medido
con `BenchmarkHashCalibration` compilado para Linux y ejecutado en el Droplet (`/tmp` es
`noexec`: usar el home). Recalibrar en PROD con su hardware.

## 6. Backups

Diarios con `booking-backup.timer` (T3.6); ver `backup-restore.md`.
