# Decisiones

Decisiones relevantes no arquitectónicas (las arquitectónicas tienen ADR en `docs/adr/`).

| Fecha | Decisión | Motivo | Estado |
|---|---|---|---|
| 2026-11-18 | Código e identificadores en inglés; documentación en español | Convención del ecosistema Go; documentación para el equipo | Propuesto |
| 2026-11-18 | Git inicializado en `booking-platform-backend/` con rama `main` | Regla 10 (alcance) + petición de versionar todo para GitHub; ver ADR-0014 | Aplicado (provisional) |
| 2026-11-18 | Conventional Commits + trunk-based + squash merge | Historial legible, compatible con releases automáticos | Propuesto |
| 2026-11-18 | Migraciones en `db/migrations/` en lugar de `migrations/` | Agrupar con `db/queries/` de SQLC | Propuesto |
| 2026-11-18 | `Taskfile.yml` (go-task) en lugar de Makefile | Multiplataforma (desarrollo en Windows, CI en Linux) | Propuesto |
| 2026-11-18 | Integración con `testcontainers-go` (imagen postgis) | PG real y aislado por suite, igual local y CI | Propuesto |
| 2026-11-18 | Imágenes en GHCR | Gratuito con GitHub, integrado con Actions | Propuesto |
| 2026-11-18 | Búsqueda de negocios por ciudad en MVP (PostGIS listo para cercanía futura) | Simplicidad | Propuesto |
| 2026-11-18 | Sin auto-cancelación de citas pendientes en MVP | Regla del prompt; requiere política aprobada | Propuesto |
| 2026-11-18 | Recurso de otro negocio → 404 | Evita enumeración (IDOR) | Propuesto |
