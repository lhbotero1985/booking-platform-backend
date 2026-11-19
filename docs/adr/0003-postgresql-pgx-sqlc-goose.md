# ADR-0003: PostgreSQL + PostGIS con pgx, SQLC y Goose

- Estado: Aceptado (aprobación Fase 0, 2026-11-18) · Fecha: 2026-11-18

## Decisión
- PostgreSQL (estable vigente, mínimo 16) con `postgis`, `btree_gist`, `citext`.
- Driver `pgx/v5` con `pgxpool`.
- SQLC para generar código tipado desde SQL (sin ORM).
- Goose para migraciones SQL embebidas, ejecutadas como paso explícito de despliegue.

## Alternativas
- ORM (GORM/ent): oculta SQL, dificulta exclusion constraints y consultas de rango.
- golang-migrate: equivalente; Goose elegido por soporte de Go embebido y por el prompt.

## Consecuencias
- (+) SQL explícito, revisable y parametrizado; rendimiento predecible.
- (−) Más SQL escrito a mano; mitigado con SQLC.
- PostGIS se instala desde Fase 1, aunque la búsqueda por cercanía no es requisito del MVP
  (la búsqueda MVP es por ciudad).
