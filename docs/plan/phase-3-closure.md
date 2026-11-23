# Cierre de la Fase 3 — Infraestructura QA en DigitalOcean

Fecha: 2026-11-23 · Estado: **✅ Aprobada por el usuario (2026-11-23)**

QA en línea: **https://booking-platform-col.duckdns.org** (Droplet `booking-qa`, 1 vCPU / 1 GB +
2 GB swap, nyc3). Costo: solo el Droplet; ningún otro recurso pago.

## 1. Entregables

| Tarea | Entregable | Estado | Evidencia |
|---|---|---|---|
| T3.1 | `deploy/server/bootstrap.sh`: usuario `deploy`, SSH solo con llave (root y contraseñas deshabilitados), ufw (22/80/443), fail2ban, actualizaciones automáticas con reinicio 04:30 UTC, swap 2 GB, Docker endurecido | ✅ | PR #23 |
| T3.2 | Stack Compose QA: PostGIS ajustado a 1 GB, api, worker, Mailpit solo local; roles `app_migrator` (DDL) / `app_runtime` (DML); secretos generados en el servidor | ✅ | PR #24 |
| T3.3 | Caddy 2.11.9: Let's Encrypt, HSTS, HTTP/3, TLS ≥ 1.2, límite de cuerpo; IP real del cliente solo desde el proxy de confianza | ✅ | PR #25 |
| T3.4 | `release-qa.yml`: build, Trivy, SBOM CycloneDX, push de la imagen escaneada a GHCR privado por digest, retención de 10 | ✅ | PR #26, #27, #29 |
| T3.5 | Despliegue automático en cada merge a `qa`: llave de CI restringida a un comando, token efímero, stack dentro de la imagen, backup previo, migraciones, smoke tests, rollback automático | ✅ | PR #26, #29 |
| T3.6 | `booking-backup` + timer systemd: diarios (7) y semanales (4), verificación de restauración semanal, restauración en sitio | ✅ | PR #28 |
| T3.7 | `scripts/e2e-qa.sh` (52 verificaciones), calibración de Argon2id (P27), manuales de operación | ✅ | PR #30 |
| T3.8 | Este documento; `release-tag.yml` (re-etiquetado del digest validado) | ✅ | `feature/t3.8-phase-3-closure` |

Manuales: `docs/operations/server-bootstrap.md`, `qa-deployment.md`, `backup-restore.md`,
`rollback.md`.

## 2. Pruebas y verificaciones (reales, en QA)

| Verificación | Resultado | Evidencia (`docs/testing/test-results.md`) |
|---|---|---|
| Endurecimiento: root y contraseñas rechazados, solo 22/80/443 abiertos desde fuera | ✅ | T3.1, T3.3 |
| Stack sano, puertos internos solo en `127.0.0.1`, privilegios de BD mínimos | ✅ | T3.2 |
| Certificado válido, 308 a HTTPS, HSTS, sin `Server`, TLS ≤ 1.1 rechazado, 413 con 3 MB | ✅ | T3.3 |
| IP real en logs y `audit_log` pese a `X-Forwarded-For` falso | ✅ | T3.3, T3.7 |
| Despliegue automático (4 ejecuciones exitosas: `5070757`, `9b169a9`, `d0f48b1`, + este cierre) | ✅ | T3.4–T3.7 |
| Rollback automático (versión incorrecta → vuelve a la anterior, job en rojo) | ✅ | T3.5 |
| Llave de CI: sin shell, sin túneles, entradas inválidas rechazadas | ✅ | T3.5 |
| Backup diario, verificación de restauración, archivos truncados/alterados rechazados, restauración en sitio con permisos intactos | ✅ | T3.6 |
| E2E 52/52 por HTTPS público (antes y después de la calibración de Argon2) | ✅ | T3.7 |
| Argon2id: 64 MiB, t=2 → 222–262 ms; rehash automático verificado | ✅ | T3.7 |
| CI en cada PR: build, unitarias con `-race`, integración, golangci-lint, sqlc, shellcheck, govulncheck, gitleaks, Trivy config + imagen, extracción del stack | ✅ | PR #23–#30 |

## 3. Checklist de seguridad de la infraestructura

| Control | Estado |
|---|---|
| SSH: solo llave, sin root, `MaxAuthTries 3`, `AllowUsers deploy`, fail2ban | ✅ |
| Firewall: ufw deny por defecto; Docker solo publica Caddy | ✅ (sin Cloud Firewall de DO: P36) |
| TLS automático, HSTS, HTTP/3; cabeceras de seguridad desde la API | ✅ |
| Contenedores: `read_only`, `cap_drop: ALL`, `no-new-privileges`, límites de memoria y PIDs, imágenes fijadas por digest | ✅ |
| Base de datos: sin exposición, roles de mínimo privilegio, SCRAM | ✅ |
| Secretos: generados en el servidor (`.env` 0600), nunca en Git ni en CI; llave de CI solo en el Environment `qa` | ✅ |
| Cadena de suministro: acciones fijadas por SHA, escáneres por digest, imagen escaneada = imagen publicada, SBOM | ✅ (firma cosign: Fase 13) |
| Despliegue: solo imágenes de este repo por digest; registro de despliegues; backup previo | ✅ |
| Actualizaciones de seguridad automáticas del SO | ✅ |
| Datos: solo sintéticos en QA; Mailpit (ningún correo real sale) | ✅ |

## 4. Riesgos aceptados y deuda

| # | Tema | Decisión |
|---|---|---|
| P34 | Droplet de 1 GB | Suficiente hoy (≈600 MB usados); subir a 2 GB (US$ 14/mes) si la carga lo exige |
| P36 | Sin Cloud Firewall de DO | Solo ufw |
| P37 | Sin protección de ramas (plan Free) | Regla de proceso: solo merges de PR |
| P38 | Backups en el mismo disco | Aceptado en QA; copia externa obligatoria en PROD |
| — | SSH (22) abierto a Internet | Mitigado con llave + fail2ban; restringir por IP si se usa Cloud Firewall |
| — | `deploy` con sudo sin contraseña | Necesario para administración; la llave de CI no puede usarlo (comando forzado) |
| — | Monitoreo y alertas (uptime, disco, fallos del timer de backup) | Pendiente (Fase 12/13) |
| — | Firma de imágenes (cosign) | Fase 13 |
| — | Actualizar los scripts del servidor (`booking-deploy`, `booking-backup`) es manual | Documentado; la llave de CI no puede modificarlos a propósito |

## 5. Release

**Decisión del usuario (2026-11-23): el paso a `main` se hará cuando se vaya a producción**
(Fase 13), no al cerrar esta fase. Hasta entonces `qa` acumula las fases validadas.

Mecanismo listo para ese momento: PR `qa` → `main` (merge commit, ADR-0017) y tag `vX.Y.Z` en
el merge commit; `release-tag.yml` re-etiqueta la imagen validada en QA (`qa-<sha>`) sin
reconstruirla (mismo digest; la retención de GHCR conserva las versiones con tag de release).
Mientras tanto, la retención de GHCR (10 `qa-*`) puede borrar imágenes antiguas: la release se
hará desde la imagen del último despliegue validado en QA.

## 6. Aprobación

- [x] Usuario aprueba el cierre de la Fase 3 (2026-11-23).
- [x] PR `qa` → `main`: **pospuesto hasta el paso a producción** (decisión del usuario).
