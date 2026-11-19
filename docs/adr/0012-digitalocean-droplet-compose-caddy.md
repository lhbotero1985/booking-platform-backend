# ADR-0012: DigitalOcean Droplet + Docker Compose + Caddy

- Estado: Propuesto · Fecha: 2026-11-18

## Decisión
Un Droplet Ubuntu LTS exclusivo por ambiente (QA en Fase 3, PROD en Fase 13) con Docker
Compose: `caddy` (TLS automático), `api`, `worker`, `postgres+postgis` (datos en DO Volume),
backups cifrados a DO Spaces. Imágenes en GHCR. Despliegue por GitHub Actions vía SSH.

## Alternativas
- App Platform, DOKS: más costo/complejidad.
- Nginx + certbot: más configuración que Caddy.
- DO Managed PostgreSQL: a evaluar para PROD (Fase 13).

## Consecuencias
- (+) Bajo costo, control total, reproducible localmente.
- (−) Operamos PostgreSQL nosotros (backups, actualizaciones); mitigado con procedimientos
  probados en QA.
