# Rollback

Estado: **Diseño — se implementa en la Fase 3.**

## Principios

- Cada despliegue referencia una imagen por **digest**; el digest anterior queda registrado.
- Las migraciones son compatibles hacia atrás (expand/contract), de modo que la versión anterior
  de la aplicación funciona con el esquema nuevo. **El rollback de aplicación no revierte el
  esquema.**
- Revertir el esquema solo con restauración de backup y autorización explícita.

## Procedimiento (aplicación)

1. Identificar el digest anterior en el registro de despliegues.
2. Ejecutar el workflow de despliegue con ese digest (o, manualmente, actualizar el digest en el
   `.env` de despliegue y `docker compose up -d api worker`).
3. Ejecutar smoke tests.
4. Registrar el rollback y abrir incidente con causa raíz.

## Rollback automático

El workflow de despliegue revierte al digest anterior si los smoke tests fallan.
