# Decisiones

Decisiones relevantes no arquitectónicas (las arquitectónicas tienen ADR en `docs/adr/`).

| Fecha | Decisión | Motivo | Estado |
|---|---|---|---|
| 2026-11-18 | Código e identificadores en inglés; documentación en español | Convención del ecosistema Go; documentación para el equipo | Propuesto |
| 2026-11-18 | Git en `booking-platform-backend/` → movido a la raíz (monorepo) → **devuelto al backend** | Aclaración del usuario: dos repos en GitHub (backend y frontend). ADR-0016 reemplaza a ADR-0014 | Aplicado |
| 2026-11-18 | Repositorios GitHub **privados** | Decisión del usuario (P2) | Aceptado |
| 2026-11-18 | Instalar Go y herramientas en la máquina local | Autorización del usuario (P3) | Aplicado |
| 2026-11-18 | `booking-platform.code-workspace` y `README.md` en la raíz del workspace, sin versionar | No pertenecen a ningún repo (ADR-0016) | Aplicado |
| 2026-11-18 | Fase 0 aprobada; ADR 0001–0015 aceptados (0014 luego reemplazado) | Aprobación del usuario | Aceptado |
| 2026-11-18 | Ruta del módulo Go `github.com/lhbotero1985/booking-platform-backend` | Usuario de GitHub confirmado | Aplicado |
| 2026-11-18 | Conventional Commits + trunk-based + squash merge | Historial legible, compatible con releases automáticos | Propuesto |
| 2026-11-18 | Migraciones en `db/migrations/` en lugar de `migrations/` | Agrupar con `db/queries/` de SQLC | Propuesto |
| 2026-11-18 | `Taskfile.yml` (go-task) en lugar de Makefile | Multiplataforma (desarrollo en Windows, CI en Linux) | Propuesto |
| 2026-11-18 | Integración con `testcontainers-go` (imagen postgis) | PG real y aislado por suite, igual local y CI | Propuesto |
| 2026-11-19 | Puerto DEV de PostgreSQL 55432 | PostgreSQL nativo ocupa 5432 en la máquina de desarrollo | Aplicado |
| 2026-11-19 | Trivy fijado a 0.58.1 con DB desde GHCR | mirror.gcr.io inalcanzable por IPv6 en la red local | Aplicado |
| 2026-11-19 | La API arranca sin BD; `/readyz` (BD + migraciones) decide si recibe tráfico | Resiliencia ante arranques desordenados | Aplicado |
| 2026-11-19 | `migrate down` prohibido en QA/PROD | Forward-only; revertir = restaurar backup | Aplicado |
| 2026-11-19 | El Down de 00001 no elimina postgis | Imagen/proveedor la preinstalan con extensiones dependientes | Aplicado |
| 2026-11-19 | Roles `app_migrator`/`app_runtime` diferidos a Fase 3 | Se crean con la infraestructura QA; DEV usa un solo rol | Pendiente |
| 2026-11-19 | Una rama `feat/…` por tarea + PR a `main`; sin rama `qa` | Trunk-based (ADR-0013): QA es ambiente, no rama | Aplicado |
| 2026-11-18 | Imágenes en GHCR | Gratuito con GitHub, integrado con Actions | Propuesto |
| 2026-11-18 | Búsqueda de negocios por ciudad en MVP (PostGIS listo para cercanía futura) | Simplicidad | Propuesto |
| 2026-11-18 | Sin auto-cancelación de citas pendientes en MVP | Regla del prompt; requiere política aprobada | Propuesto |
| 2026-11-18 | Recurso de otro negocio → 404 | Evita enumeración (IDOR) | Propuesto |
