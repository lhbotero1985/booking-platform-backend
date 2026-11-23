# Rollback

Estado: **QA implementado (T3.5, 2026-11-23).** PROD en la Fase 13.

## Principios

- Cada despliegue referencia una imagen por **digest**; el digest anterior queda registrado.
- Las migraciones son compatibles hacia atrás (expand/contract), de modo que la versión anterior
  de la aplicación funciona con el esquema nuevo. **El rollback de aplicación no revierte el
  esquema.**
- Revertir el esquema solo con restauración de backup y autorización explícita.

## Procedimiento (aplicación)

1. Identificar el digest anterior en `/opt/booking/qa/deployments.log` (o en el resumen del
   workflow `release-qa`).
2. Re-desplegarlo con el script del servidor (ver `qa-deployment.md`, "Despliegue manual"): hace
   backup, smoke tests y rollback propio si falla. Alternativa sin registro: imagen local
   anterior en `API_IMAGE` del `.env` y `docker compose up -d`.
3. Ejecutar smoke tests.
4. Registrar el rollback y abrir incidente con causa raíz.

## Rollback automático

`booking-deploy` revierte si fallan las migraciones, el arranque o los smoke tests: restaura los
archivos del stack desde `rollback/` y la imagen anterior, repite los smoke tests y registra
`rolled-back` (o `rollback-failed`, que exige intervención manual). El job queda en rojo.
Probado en QA el 2026-11-23 (versión esperada incorrecta → rollback a `qa-2d3f46c` en 4 s).

Restaurar el esquema/datos: `backups/predeploy-*.dump` (`pg_restore`), solo con autorización
explícita (ver `backup-restore.md`).
