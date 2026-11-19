# booking-platform-backend

Backend (Go) de una plataforma SaaS multiempresa de reservas para barberías, salones de belleza
y negocios de servicios con cita.

> Estado: **Fase 0 — Arquitectura**. El repositorio contiene solo documentación de diseño.
> No hay código funcional todavía.

## Documentación

- Punto de entrada para colaboradores y Claude Code: [CLAUDE.md](CLAUDE.md)
- Arquitectura: [docs/architecture/overview.md](docs/architecture/overview.md)
- Decisiones (ADR): [docs/adr/](docs/adr/)
- Roadmap y progreso: [docs/plan/](docs/plan/)
- Política de seguridad: [SECURITY.md](SECURITY.md)

Repositorio independiente. El frontend vive en su propio repositorio (`booking-platform-frontend`)
y consume el contrato `api/openapi.yaml` de este repositorio.

## Stack previsto

Go · PostgreSQL/PostGIS · pgx · SQLC · Goose · River · OpenAPI 3.1 · Docker · GitHub Actions ·
DigitalOcean.
