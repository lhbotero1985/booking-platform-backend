# booking-platform-backend

Backend (Go) de una plataforma SaaS multiempresa de reservas para barberías, salones de belleza
y negocios de servicios con cita.

> Estado: Fases 0 a 7 aprobadas y en QA (`docs/plan/roadmap.md`); Fase 8 (portal del negocio)
> en el repositorio del frontend. Detalle de cada tarea en `docs/plan/progress.md`.

## Qué hace hoy

- **Cuentas y seguridad:** registro con correo verificado, sesiones (Bearer y cookie con CSRF),
  roles por negocio (OWNER, PROFESSIONAL) y administrador de la plataforma, auditoría.
- **Negocios:** alta autoservicio con demo de 7 días (ADR-0018), hasta `OWNED_BUSINESS_LIMIT`
  negocios por cuenta (P-11), ubicación por ciudad, políticas de reserva, **logo y portada**
  (ADR-0020), búsqueda y página pública.
- **Profesionales y catálogo:** horarios, descansos, ausencias, servicios con precio y duración,
  **portafolio de hasta 10 imágenes** por profesional (ADR-0020).
- **Reservas:** motor de disponibilidad, citas sin doble reserva (ADR-0004), estados,
  reprogramación con límite, agenda, citas manuales, clientes del negocio, estadísticas (P-12).
- **Reseñas (ADR-0021):** el cliente de una cita completada califica al negocio y al profesional
  (1 a 5) con comentario, hasta 30 días después; el dueño responde; solo el administrador oculta;
  promedios en la página pública, la búsqueda y el catálogo; correo de invitación al completar la
  cita.
- **Notificaciones:** WhatsApp (Meta Cloud API) y correo, recordatorios, enlaces de confirmar,
  reagendar y cancelar, preferencias y topes de WhatsApp (ADR-0019).

El contrato completo está en `api/openapi.yaml` (120 operaciones).

## Documentación

- Punto de entrada para colaboradores y Claude Code: [CLAUDE.md](CLAUDE.md)
- Arquitectura: [docs/architecture/overview.md](docs/architecture/overview.md)
- Decisiones (ADR): [docs/adr/](docs/adr/)
- Roadmap y progreso: [docs/plan/](docs/plan/)
- Política de seguridad: [SECURITY.md](SECURITY.md)

Repositorio independiente. El frontend vive en su propio repositorio (`booking-platform-frontend`)
y consume el contrato `api/openapi.yaml` de este repositorio.

## Stack

Go · PostgreSQL/PostGIS · pgx · SQLC · Goose · River · OpenAPI 3.1 · Docker · GitHub Actions ·
DigitalOcean.
