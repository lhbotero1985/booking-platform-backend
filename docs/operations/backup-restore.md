# Backup y restauración

Estado: **QA implementado (T3.6, 2026-11-23).** PROD en la Fase 13.

## 1. QA hoy

| Aspecto | QA |
|---|---|
| Método | `pg_dump -Fc` (superusuario `booking_admin`, incluye dueños, permisos y ACL de la base) |
| Programación | `booking-backup.timer` (systemd): diario 03:15 UTC (+ hasta 10 min aleatorios), `Persistent=true` |
| Destino | `/opt/booking/qa/backups/` en el mismo Droplet (0700, archivos 0600, dueño `deploy`) |
| Retención | 7 diarios (`daily-*`), 4 semanales (`weekly-*`, copia del domingo), 5 previos a despliegue (`predeploy-*`, `booking-deploy`), 3 previos a restauración (`prerestore-*`) |
| Integridad | `pg_restore --list` tras cada dump (archivo truncado = error) y `.sha256` por archivo |
| Prueba de restauración | **Automática cada domingo** (`verify`: restaura en una base temporal y compara tablas, filas y versión de migraciones); manual con `booking-backup verify` |
| Coordinación | Comparte el lock de `booking-deploy`: nunca hace dump durante una migración |
| Estado | `backups/last-backup` y `journalctl -u booking-backup` |

Fuente: `deploy/server/backup.sh` (→ `/usr/local/bin/booking-backup`) y
`deploy/server/systemd/booking-backup.{service,timer}`.

### Riesgo aceptado (P38)

Los backups están **en el mismo disco** que la base: protegen contra errores (migración mala,
borrado accidental) pero **no** contra la pérdida del Droplet. Copia externa sin costo: bajarlos
manualmente (`scp`, ver §4). Para PROD se requiere copia externa cifrada (DO Spaces u otro, con
autorización de costo).

## 2. Instalación / actualización en el servidor

```bash
ssh deploy@HOST 'sudo install -m 0755 -o root -g root /dev/stdin /usr/local/bin/booking-backup' < deploy/server/backup.sh
ssh deploy@HOST 'sudo install -m 0644 /dev/stdin /etc/systemd/system/booking-backup.service' < deploy/server/systemd/booking-backup.service
ssh deploy@HOST 'sudo install -m 0644 /dev/stdin /etc/systemd/system/booking-backup.timer' < deploy/server/systemd/booking-backup.timer
ssh deploy@HOST 'sudo systemctl daemon-reload && sudo systemctl enable --now booking-backup.timer'
```

## 3. Operación

```bash
ssh deploy@HOST 'systemctl list-timers booking-backup.timer; cat /opt/booking/qa/backups/last-backup'
ssh deploy@HOST 'sudo systemctl start booking-backup.service'      # backup inmediato
ssh deploy@HOST 'booking-backup verify'                              # prueba de restauración (último diario)
ssh deploy@HOST 'booking-backup verify /opt/booking/qa/backups/<archivo>.dump'
```

## 4. Restauración (reemplaza la base viva)

**Solo con autorización explícita.** Pérdida de datos desde el backup elegido.

```bash
ssh deploy@HOST 'ls -lt /opt/booking/qa/backups'
ssh deploy@HOST 'booking-backup restore /opt/booking/qa/backups/<archivo>.dump --yes'
```

El comando:
1. Verifica el `.sha256`.
2. Hace un dump de seguridad (`prerestore-*`) del estado actual.
3. Detiene `api` y `worker` (Caddy reintenta 10 s; después responde 502).
4. `pg_restore --clean --create`: recrea la base con dueño, permisos y ACL originales.
5. Arranca `api` y `worker` y espera `/readyz`.

Si `pg_restore` falla, vuelve a arrancar los servicios e indica el dump de seguridad para
recuperar el estado previo con el mismo comando.

Copia externa manual:

```bash
scp deploy@HOST:/opt/booking/qa/backups/daily-<fecha>.dump* ./   # contiene datos: guardar cifrado
```

## 5. Resultados de la prueba real (2026-11-23)

Ver `docs/testing/test-results.md` (T3.6): restauración en sitio con usuario marcador
(desaparece tras restaurar), dueños/privilegios/ACL intactos, API escribiendo de nuevo; caída
≈5 s.

## 6. PROD (Fase 13, diseño)

| Aspecto | PROD |
|---|---|
| Método | `pg_dump -Fc` diario + (❓) WAL/PITR o Managed PG |
| Cifrado | `age` con llave pública en el servidor; privada fuera del servidor |
| Destino | Copia externa (DO Spaces, bucket PROD, credenciales PROD) — con autorización de costo |
| Retención | 30 días diarios + 12 mensuales |
| Prueba de restauración | Mensual, en ambiente aislado |
| Backup pre-migración | Siempre |

Objetivos iniciales PROD: RPO ≤ 24 h (≤ 5 min si se adopta PITR), RTO ≤ 2 h.
