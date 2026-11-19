# Architecture Decision Records

Formato: contexto, decisión, alternativas, consecuencias. Estados: `Propuesto`, `Aceptado`,
`Reemplazado por ADR-XXXX`, `Rechazado`. Los ADR de la Fase 0 fueron aceptados el 2026-11-18 al aprobarse
la fase.

| ADR | Título | Estado |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Registrar decisiones con ADR | Aceptado |
| [0002](0002-modular-monolith-ddd-clean-architecture.md) | Monolito modular Go con DDD y Clean Architecture | Aceptado |
| [0003](0003-postgresql-pgx-sqlc-goose.md) | PostgreSQL + PostGIS con pgx, SQLC y Goose | Aceptado |
| [0004](0004-double-booking-prevention.md) | Prevención de doble reserva | Aceptado |
| [0005](0005-time-handling-utc-iana.md) | Manejo de tiempo: UTC + zona IANA | Aceptado |
| [0006](0006-authentication-opaque-tokens-argon2id.md) | Autenticación con tokens opacos y Argon2id | Aceptado |
| [0007](0007-authorization-rbac-tenant-isolation.md) | Autorización RBAC y aislamiento multiempresa | Aceptado |
| [0008](0008-background-jobs-river.md) | Jobs en segundo plano con River (outbox transaccional) | Aceptado |
| [0009](0009-whatsapp-cloud-api-central-number.md) | WhatsApp Cloud API con número central | Aceptado |
| [0010](0010-push-expo.md) | Push con Expo Push Notifications | Aceptado |
| [0011](0011-rest-openapi-stdlib-router.md) | REST spec-first OpenAPI 3.1 con router stdlib | Aceptado |
| [0012](0012-digitalocean-droplet-compose-caddy.md) | DigitalOcean Droplet + Docker Compose + Caddy | Aceptado |
| [0013](0013-devsecops-git-workflow.md) | DevSecOps y flujo Git | Aceptado |
| [0014](0014-repository-layout.md) | Monorepo para toda la plataforma | Reemplazado por 0016 |
| [0015](0015-identifiers-and-money.md) | Identificadores UUIDv7 y dinero en unidades menores | Aceptado |
| [0016](0016-separate-backend-frontend-repositories.md) | Repositorios separados para backend y frontend | Aceptado |
