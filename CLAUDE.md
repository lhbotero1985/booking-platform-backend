# CLAUDE.md — booking-platform-backend

Punto de entrada para Claude Code. Leer completo al iniciar cada sesión.

## Objetivo

Backend de una plataforma SaaS multiempresa de reservas para barberías, salones de belleza,
centros de uñas y negocios similares. Prioridades del MVP, en orden:

1. Confiabilidad del motor de reservas (cero doble reserva).
2. Seguridad y aislamiento multiempresa.
3. Experiencia simple para negocios, profesionales y clientes.

## Repositorio

Repositorio Git **privado e independiente**: `booking-platform-backend/` es la raíz del repo
(ADR-0016). El frontend tendrá su propio repositorio (`booking-platform-frontend`). La carpeta
`booking-platform/` es solo el workspace local (contiene `README.md` y
`booking-platform.code-workspace`, que no se versionan en ningún repo). Toda la configuración
Git/DevSecOps (`.github/`, `.gitleaks.toml`, etc.) vive en este repositorio.

## Estado actual

Ver `docs/plan/progress.md` y `docs/plan/next-task.md`. La fase vigente y lo autorizado se
registran allí; **no asumir que una fase está aprobada si no consta en `progress.md`**.

## Arquitectura (resumen)

- Monolito modular en Go, DDD + Clean Architecture + SOLID. Detalle: `docs/architecture/overview.md`.
- Bounded contexts: `identity`, `geography`, `businesses`, `professionals`, `catalog`,
  `scheduling`, `notifications`.
- Capas por módulo: `domain` → `application` → `infrastructure` / `interfaces`.
  El dominio no importa nada de infraestructura ni de otros módulos.
- Los módulos se comunican solo a través de su paquete `contract` (interfaces y DTO públicos).
- Dos binarios: `cmd/api` (HTTP) y `cmd/worker` (River).

## Stack

Go (stdlib `net/http`, `log/slog`) · PostgreSQL + PostGIS · pgx v5 · SQLC · Goose · River ·
REST + OpenAPI 3.1 · Docker / Docker Compose · GitHub Actions · DigitalOcean (Droplets) · Caddy.

## Convenciones

- Idioma: código, identificadores y commits en **inglés**; documentación en **español**.
- IDs: UUIDv7 generados en la aplicación.
- Tiempo: `timestamptz` en UTC; zona horaria IANA por negocio; nunca `time.Now()` directo en
  dominio (inyectar `Clock`).
- Dinero: enteros en unidades menores + código ISO 4217.
- Errores HTTP: RFC 9457 (`application/problem+json`).
- Logs: JSON estructurado con `slog`; nunca registrar contraseñas, tokens, teléfonos completos
  ni cuerpos de webhooks.
- SQL: solo consultas parametrizadas vía SQLC. Toda consulta de datos de negocio filtra por
  `business_id`.
- Commits: Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, `test:`, `ci:`, `refactor:`).
- Ramas: `main` protegida; trabajo en `feat/…`, `fix/…`, `docs/…`; PR obligatorio con CI verde.

## Reglas obligatorias (resumen del prompt maestro)

1. Trabajar por fases y tareas pequeñas con criterios de aceptación.
2. **No avanzar de fase ni a tareas que requieran autorización sin aprobación explícita.**
3. Antes de implementar: objetivo, archivos afectados, decisiones técnicas (formato en sección
   "Formato por tarea").
4. Evitar sobreingeniería. Sin microservicios, Kubernetes ni Kafka.
5. Ejecutar pruebas (unitarias, integración, seguridad, concurrencia según aplique).
   **Nunca declarar pruebas exitosas sin ejecutarlas; registrar resultados reales en
   `docs/testing/test-results.md`.**
6. No declarar terminada una tarea con errores críticos.
7. Actualizar Markdown tras cada tarea (`progress.md`, `next-task.md`, decisiones, ADR).
8. No modificar arquitectura aprobada sin consultar.
9. No ejecutar comandos destructivos, no crear recursos pagos, no hacer **push** ni desplegar
   sin autorización. Commits locales: solo cuando el usuario lo autorice para la tarea.
10. No almacenar secretos en Git (gitleaks en pre-commit y CI). No usar datos reales fuera de PROD.
11. No modificar archivos fuera de `booking-platform-backend` sin autorización explícita.

## Ubicación de la documentación

| Tema | Archivo |
|---|---|
| Arquitectura general | `docs/architecture/overview.md` |
| Modelo de dominio | `docs/architecture/domain-model.md` |
| Modelo de datos | `docs/architecture/database.md` |
| Ciclo de vida de citas | `docs/architecture/booking-lifecycle.md` |
| Motor de disponibilidad y concurrencia | `docs/architecture/availability-engine.md` |
| Seguridad (authN/authZ) | `docs/architecture/security.md` |
| Notificaciones | `docs/architecture/notifications.md` |
| Diseño de API | `docs/architecture/api-design.md` |
| DevSecOps y CI/CD | `docs/architecture/devsecops.md` |
| Ambientes | `docs/architecture/environments.md` |
| Despliegue / DigitalOcean | `docs/architecture/deployment.md` |
| ADR | `docs/adr/` |
| Plan, progreso, siguiente tarea | `docs/plan/` |
| Pruebas | `docs/testing/` |
| Operación (QA/PROD, backups, rollback) | `docs/operations/` |

## Procedimiento de inicio de sesión

1. Leer este archivo.
2. Leer `docs/plan/roadmap.md`, `docs/plan/progress.md`, `docs/plan/next-task.md`.
3. Leer los ADR relevantes a la tarea.
4. Inspeccionar el repositorio y `git status` / `git log --oneline -10`.
5. Presentar el estado al usuario.
6. Continuar solo con la tarea autorizada.

## Procedimiento de cierre de sesión

1. Actualizar `docs/plan/progress.md` (fecha, fase, tarea, estado, archivos, pruebas, resultados,
   problemas, próximos pasos).
2. Actualizar `docs/plan/next-task.md`.
3. Registrar decisiones en `docs/plan/decisions.md` y crear ADR si son arquitectónicas.
4. Registrar pruebas reales en `docs/testing/test-results.md`.
5. Registrar preguntas abiertas en `docs/plan/pending-questions.md`.

## Formato por tarea

**Antes:** FASE · TAREA · OBJETIVO · ARCHIVOS · IMPLEMENTACIÓN · PRUEBAS · CRITERIOS DE ACEPTACIÓN.
**Después:** RESULTADO · ARCHIVOS · PRUEBAS (comandos y resultados reales) · RIESGOS ·
DOCUMENTACIÓN · SIGUIENTE TAREA.

## Comandos

Toolchain local: Go 1.27.4, golangci-lint 2.x, govulncheck, sqlc, goose, Task (go-task).
Ruta del módulo Go: `github.com/lhbotero1985/booking-platform-backend`.

| Comando | Qué hace |
|---|---|
| `task check` | build + vet + lint + test + vuln (ejecutar antes de cerrar cada tarea) |
| `task build` | Compila `cmd/api` y `cmd/worker` en `bin/` con versión inyectada |
| `task test` / `task test:cover` | Pruebas unitarias / con cobertura |
| `task test:race` | Detector de carreras — requiere cgo; **en este Windows no hay gcc, corre en CI** |
| `task lint` / `task fmt` | golangci-lint (gosec, depguard de arquitectura…) / formateo |
| `task vuln` | govulncheck |
| `task secrets` | gitleaks vía Docker (requiere Docker Desktop en ejecución) |

Pendientes de tareas posteriores: `task up` (T1.3), `task test:integration` (T1.4),
`task sqlc` (T1.5).
