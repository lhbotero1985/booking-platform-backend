# Roadmap

Orden obligatorio. **No se avanza de fase sin aprobación explícita del usuario.**

| Fase | Nombre | Alcance | Estado |
|---|---|---|---|
| 0 | Arquitectura | Diseño, ADR, plan | ✅ Aprobada 2026-11-18 |
| 1 | Cimientos técnicos | Proyecto Go, Docker, PG, SQLC, Goose, River, OpenAPI, CI | ✅ Aprobada 2026-11-19 (`phase-1-closure.md`) |
| 2 | Autenticación y autorización | Usuarios, sesiones, Argon2id, RBAC, auditoría | ✅ Aprobada 2026-11-19 (`phase-2-closure.md`) |
| 3 | DigitalOcean QA | Droplet QA, HTTPS, backups, deploy QA (requiere autorización de costos) | 🟡 En curso (desglose aprobado 2026-11-22) |
| 4 | Geografía y negocios | Países/divisiones/ciudades, negocios, aislamiento | ⏳ |
| 5 | Profesionales y servicios | Profesionales, horarios, descansos, ausencias, catálogo | ⏳ |
| 6 | Disponibilidad y reservas | Motor de slots, citas, estados, concurrencia | ⏳ |
| 7 | WhatsApp y push | Meta Cloud API, Expo, River jobs, webhooks | ⏳ |
| 8 | Portal del negocio | Next.js | ⏳ |
| 9 | App del cliente | Expo | ⏳ |
| 10 | App del profesional | Expo | ⏳ |
| 11 | Portal administrador | Next.js | ⏳ |
| 12 | Pruebas integrales y piloto | E2E, carga, seguridad, piloto | ⏳ |
| 13 | DigitalOcean PROD | Infra productiva independiente | ⏳ |

## Fase 0 — Arquitectura (tareas)

| ID | Tarea | Estado |
|---|---|---|
| T0.1 | Diagnóstico del repositorio | ✅ |
| T0.2 | Inicialización Git y base DevSecOps (sin código funcional) | ✅ |
| T0.3 | Documentación de arquitectura, ADR, estrategia de pruebas, plan | ✅ |
| T0.4 | Revisión con el usuario, respuestas a `pending-questions.md`, ADR → Aceptado | ✅ |

## Fase 1 — Cimientos técnicos (tareas propuestas)

| ID | Tarea | Criterio de aceptación principal |
|---|---|---|
| T1.1 ✅ | Toolchain local + `go.mod` + esqueleto de carpetas + `Taskfile.yml` | `go build ./...` y `task lint` pasan en Windows y en CI |
| T1.2 ✅ | `platform`: config, logging `slog`, errores RFC 9457, router y middleware base (request-id, recover, logging, límites de body, CORS) | Pruebas unitarias de middleware; config inválida falla al iniciar |
| T1.3 ✅ | Docker: Dockerfile multi-stage no-root + `docker-compose.dev.yml` (postgis) + `.env.example` | `task up` levanta API y BD; Trivy config sin HIGH/CRITICAL |
| T1.4 ✅ | BD: pgxpool, Goose embebido, migración 00001 (extensiones, roles), `api migrate`, `/healthz`, `/readyz` | Integración: migraciones up/down/up en PG real; `/readyz` 503 sin BD |
| T1.5 ✅ | SQLC configurado + verificación `sqlc diff` en CI | Código generado al día |
| T1.6 ✅ | River: migraciones, cliente, `cmd/worker`, job de mantenimiento de ejemplo | Integración: job encolado con `InsertTx` y procesado |
| T1.7 ✅ | OpenAPI 3.1 base (`/healthz`, `/readyz`, esquema Problem) + validación de respuestas en tests | Spec válido (lint) y respuestas conformes |
| T1.8 ✅ | GitHub Actions `ci.yml` + `security.yml` (build, unit, race, integración, golangci-lint/gosec, govulncheck, gitleaks, trivy) con acciones fijadas por SHA | Pipeline verde en el primer PR |
| T1.9 ✅ | Seguridad básica + cierre de fase: documentación, test-results | Checklist de fase completo |

## Fase 2 — Autenticación y autorización (tareas)

Módulo `internal/modules/identity` (ADR-0006, ADR-0007, `docs/architecture/security.md`).

| ID | Tarea | Criterio de aceptación principal |
|---|---|---|
| T2.1 ✅ | Esqueleto del módulo `identity` (domain / application / infrastructure / interfaces / contract) + migración 00003 (`users`, `sessions`, `password_reset_tokens`, `memberships`, `audit_log`) + consultas SQLC | Migración up/down/up; reglas depguard del módulo; `sqlc diff` limpio |
| T2.2 ✅ | Dominio: `Email`, política de contraseña, `User`; hasher Argon2id (PHC, calibrado, rehash) | Unitarias de VO e invariantes; Argon2id ≈100–250 ms medido |
| T2.3 ✅ | Registro + verificación obligatoria de email (token de un solo uso, job River, SMTP/Mailpit) + reenvío + OpenAPI | Registro sin enumeración; token nunca en claro (ni en la cola); E2E con Mailpit |
| T2.4 ✅ | Login (solo cuentas verificadas, `VerifyDummy`, rehash) + sesiones con token opaco + middleware de autenticación (Bearer y cookie `__Host-session` + CSRF) + logout, logout-all, `/me` | Matriz 401: sin token, malformado, expirado, revocado, no verificado, suspendido; tiempo comparable para email inexistente |
| T2.5 ✅ | Recuperación de contraseña (token `bp_pr_` de un solo uso, reutiliza mailer SMTP y jobs) | Respuesta siempre 202; token expira y no se reutiliza; revoca sesiones |
| T2.6 ✅ | RBAC: roles, membresías, políticas por caso de uso, patrón 404 entre negocios | Matriz rol × acción; 403 mismo negocio, 404 otro negocio |
| T2.7 ✅ | Rate limiting (login, registro, recuperación) y backoff ante fuerza bruta | Pruebas de límites y de backoff |
| T2.8 ✅ | Auditoría básica (login, fallos, cambios de contraseña/membresía) | Eventos auditados sin datos sensibles |
| T2.9 ✅ | Suite de seguridad de la fase + cierre | `tests/security` ampliada; checklist de fase |
| T2.10 ✅ | Política de contraseñas: mínimo 8 (decisión P31), lista de comunes de 8–9 caracteres | Unitarias de la política; OpenAPI `minLength: 8` |

**Fase 2 aprobada por el usuario (2026-11-19).**

## Fase 3 — Infraestructura QA en DigitalOcean (tareas)

Desglose aprobado por el usuario el 2026-11-22. Droplet `booking-qa` creado por el usuario.
Costo: solo el Droplet; cualquier recurso con costo adicional requiere autorización explícita.

| ID | Tarea | Criterio de aceptación principal |
|---|---|---|
| T3.1 ✅ | Preparación y endurecimiento del servidor (`deploy/server/bootstrap.sh`): usuario `deploy`, SSH solo con llave, ufw, fail2ban, actualizaciones automáticas, swap, Docker | Root y contraseñas rechazados; solo 22 abierto; script idempotente y con shellcheck en CI |
| T3.2 ✅ | Stack QA en Compose: PostGIS ajustado a la RAM, API, worker, Mailpit en `127.0.0.1`; roles de BD `app_migrator`/`app_runtime`; secretos generados en el servidor | Stack sano; ningún puerto interno expuesto; secretos fuera de Git |
| T3.3 | Caddy con HTTPS (DNS gratuito), HSTS, IP real del cliente (proxy de confianza) | Certificado válido; rate limiting y auditoría con la IP real |
| T3.4 | Publicación de la imagen en GHCR (tag por commit, digest), Trivy y SBOM | Imagen firmable y trazable desde `qa` |
| T3.5 | Despliegue automático a QA: backup previo, migraciones, smoke tests, rollback automático | Despliegue y rollback probados |
| T3.6 | Backups diarios de PostgreSQL con retención y prueba de restauración | Restauración real verificada |
| T3.7 | Validación E2E en QA (Mailpit por túnel SSH), calibración de Argon2id (P27), manuales de operación | Flujos de la Fase 2 verdes en QA |
| T3.8 | PR `qa` → `main` y cierre de la fase | Checklist de fase aprobado |

Las fases 4–13 se desglosarán en tareas al iniciar cada una.

## Riesgos identificados

| # | Riesgo | Prob. | Impacto | Mitigación |
|---|---|---|---|---|
| R1 | Doble reserva por condición de carrera | Media | Crítico | ADR-0004: lock por profesional + exclusion constraint + pruebas de concurrencia obligatorias |
| R2 | Errores de zona horaria / DST | Media | Alto | ADR-0005, tzdata embebido, pruebas por zona y fecha de transición |
| R3 | Fuga de datos entre negocios (IDOR) | Media | Crítico | Scoping por `business_id`, 404 cross-tenant, suites de seguridad por módulo |
| R4 | Aprobación de Meta (verificación de empresa, plantillas) se demora | Alta | Medio | Iniciar trámite en paralelo antes de Fase 7; push como canal alternativo |
| R5 | Costos de WhatsApp crecen sin control | Media | Medio | Topes por negocio, solo mensajes utility, kill switch, métricas |
| R6 | PostgreSQL autogestionado: pérdida de datos | Baja | Crítico | Backups cifrados diarios fuera del Droplet, pruebas de restauración, evaluar Managed PG en PROD |
| R7 | Secretos filtrados en Git | Baja | Alto | gitleaks pre-commit + CI, push protection, `.gitignore`, rotación inmediata |
| R8 | Alcance del MVP crece (pagos, reseñas…) | Alta | Medio | Roadmap estricto; módulos futuros fuera del MVP |
| R9 | Notificaciones obsoletas (recordatorio de cita cancelada) | Media | Medio | Verificación de versión al ejecutar job + cancelación de jobs |
| R10 | Cumplimiento legal de datos personales por país | Media | Alto | Consentimiento registrado, minimización, definir países del piloto |
| R11 | Entorno local Windows sin Go/herramientas | Alta | Bajo | T1.1 instala toolchain; todo reproducible también en Docker/CI |
| R12 | Deriva entre OpenAPI y la implementación | Media | Medio | Validación de respuestas contra el spec en pruebas |
