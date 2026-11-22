# Preparación y endurecimiento del servidor

Script: `deploy/server/bootstrap.sh` (idempotente, Ubuntu 24.04). Aplica a QA y, en la Fase 13,
a PROD. Verificado con shellcheck en CI (`lint`) y localmente con `task shellcheck`.

## Servidor QA

| Dato | Valor |
|---|---|
| Droplet | `booking-qa` (ID 622296806), región `nyc3`, proyecto DO `bookink-platforma` |
| IP pública | `174.138.92.121` |
| Tamaño | `s-1vcpu-1gb-35gb-intel` (1 vCPU, 1 GB, 35 GB) · redimensión a 2 GB pendiente de confirmar precio (P34) |
| Sistema | Ubuntu 24.04.5 LTS |
| Acceso | Solo `deploy` con llave SSH (ed25519). `root` y contraseñas deshabilitados |
| DNS | Pendiente de elección (sslip.io o DuckDNS, P35) |

> En la misma cuenta existe el droplet `delivery-qa` (otro proyecto). **No se toca.**

## Qué hace el script

| Área | Configuración |
|---|---|
| Paquetes | `apt upgrade`; `ufw`, `fail2ban`, `unattended-upgrades`, `curl`, `jq` |
| Actualizaciones | Seguridad automática diaria; reinicio automático 04:30 UTC si hace falta |
| Zona horaria | UTC |
| Swap | `/swapfile` de 2 GB, `vm.swappiness=10` |
| Red (sysctl) | `rp_filter`, sin redirects, `tcp_syncookies` |
| Usuario | `deploy`: sin contraseña, llaves copiadas de `root`, `sudo` sin contraseña, grupo `docker` |
| Firewall (ufw) | Entrada denegada salvo 22/tcp, 80/tcp, 443/tcp, 443/udp |
| fail2ban | Jaula `sshd`: 5 intentos en 10 min → 1 h de bloqueo |
| Docker | Repositorio oficial; `log-driver local` (10 MB × 5), `live-restore`, `no-new-privileges`, sin `userland-proxy` |
| Directorio | `/opt/booking` (`deploy`, 0750) |
| SSH (`--lock-root`) | `PermitRootLogin no`, `PasswordAuthentication no`, `AuthenticationMethods publickey`, `MaxAuthTries 3`, `AllowUsers deploy`; validado con `sshd -t` y `sshd -T` antes de recargar |

## Procedimiento

```bash
# 1. Preparar (nunca bloquea root: un fallo no deja fuera al operador)
ssh root@HOST 'bash -s' < deploy/server/bootstrap.sh
# 2. Comprobar acceso del usuario deploy
ssh deploy@HOST 'id; sudo -n true; docker run --rm hello-world'
# 3. Bloquear root y contraseñas (se niega si deploy no tiene llaves)
ssh root@HOST 'bash -s -- --lock-root' < deploy/server/bootstrap.sh
```

Re-ejecuciones posteriores (ya sin root): `ssh deploy@HOST 'sudo bash -s' < deploy/server/bootstrap.sh`.

## Advertencias

- **Docker publica puertos por fuera de ufw** (reglas iptables propias). Regla del proyecto: solo
  Caddy publica puertos (80/443); PostgreSQL, API, worker y Mailpit nunca publican puertos, o se
  enlazan a `127.0.0.1` (Mailpit, accesible solo por túnel SSH).
- El grupo `docker` equivale a root. `deploy` es el único usuario; las llaves de CI (T3.5) se
  restringirán con `command=`/`from=` en `authorized_keys` cuando se agreguen.
- `sudo` sin contraseña: aceptable porque no existen contraseñas y el acceso es solo por llave.
- Si se pierde la llave: consola de recuperación de DigitalOcean (Droplet → Access → Recovery Console).
