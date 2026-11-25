# Roadmap

Orden obligatorio. **No se avanza de fase sin aprobación explícita del usuario.**

| Fase | Nombre | Alcance | Estado |
|---|---|---|---|
| 0 | Arquitectura | Diseño, ADR, plan | ✅ Aprobada 2026-11-18 |
| 1 | Cimientos técnicos | Proyecto Go, Docker, PG, SQLC, Goose, River, OpenAPI, CI | ✅ Aprobada 2026-11-19 (`phase-1-closure.md`) |
| 2 | Autenticación y autorización | Usuarios, sesiones, Argon2id, RBAC, auditoría | ✅ Aprobada 2026-11-19 (`phase-2-closure.md`) |
| 3 | DigitalOcean QA | Droplet QA, HTTPS, backups, deploy QA (requiere autorización de costos) | ✅ Aprobada 2026-11-23 (`phase-3-closure.md`) |
| 4 | Geografía y negocios | Países/divisiones/ciudades, negocios, aislamiento | ✅ Aprobada 2026-11-24 (`phase-4-closure.md`) |
| 5 | Profesionales y servicios | Profesionales, horarios, descansos, ausencias, catálogo | ✅ Aprobada 2026-11-24 (`phase-5-closure.md`) |
| 6 | Disponibilidad y reservas | Motor de slots, citas, estados, concurrencia | ✅ Aprobada 2026-11-24 (`phase-6-closure.md`) |
| 7 | WhatsApp y push | Meta Cloud API, Expo, River jobs, webhooks | ⏳ |
| 8 | Portal del negocio | Next.js | ⏳ |
| 9 | App del cliente | Expo | ⏳ |
| 10 | App del profesional | Expo | ⏳ |
| 11 | Portal administrador | Next.js | ⏳ |
| 11B | Suscripciones y pagos | Proveedor de pagos, planes, cobro recurrente, webhooks, reactivación, retención 30 días, facturación (ADR-0018) | ⏳ (aprobada 2026-11-24) |
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
| T3.3 ✅ | Caddy con HTTPS (DNS gratuito), HSTS, IP real del cliente (proxy de confianza) | Certificado válido; rate limiting y auditoría con la IP real |
| T3.4 ✅ | Publicación de la imagen en GHCR (tag por commit, digest), Trivy y SBOM | Imagen firmable y trazable desde `qa` |
| T3.5 ✅ | Despliegue automático a QA: backup previo, migraciones, smoke tests, rollback automático | Despliegue y rollback probados |
| T3.6 ✅ | Backups diarios de PostgreSQL con retención y prueba de restauración | Restauración real verificada |
| T3.7 ✅ | Validación E2E en QA (Mailpit por túnel SSH), calibración de Argon2id (P27), manuales de operación | Flujos de la Fase 2 verdes en QA |
| T3.8 ✅ | Cierre de la fase (`phase-3-closure.md`, `release-tag.yml`); PR `qa` → `main` pospuesto hasta producción (decisión del usuario) | Checklist de fase aprobado |

## Fase 4 — Geografía y negocios (tareas)

Desglose aprobado por el usuario el 2026-11-23. Decisiones: piloto Colombia con Valle del Cauca
(Cali, Yumbo) y Antioquia (Medellín, Itagüí) (P5); miembros agregados por email de un usuario
registrado. **Ajuste 2026-11-24 (ADR-0018):** además del alta por el administrador, el dueño crea
su negocio con una demo de 7 días (T4.3b); `PLATFORM_ADMIN` puede todo sobre todos los negocios.

| ID | Tarea | Criterio de aceptación principal |
|---|---|---|
| T4.1 ✅ | Geografía: países, divisiones, ciudades (zona horaria), datos iniciales; lectura pública, escritura `PLATFORM_ADMIN` auditada; comando `api admin grant-platform-admin` | Datos iniciales en QA; no admin → 403; auditoría en la misma transacción |
| T4.2 ✅ | Dominio y persistencia de negocios: slug, zona horaria, moneda, ubicación, políticas, avisos; FK de membresías | Ciudad ∈ división ∈ país; migración e integración |
| T4.3 ✅ | Alta por el administrador como opción de soporte (nace `ACTIVE` con demo, igual que el autoservicio); suspender/reactivar | Negocio + OWNER atómicos; suspendido fuera de búsqueda |
| T4.3b ✅ | Alta autoservicio con demo de 7 días: módulo `subscriptions` (`TRIALING`…), una demo por cuenta, vencimiento automático (River), extensión por el administrador | Negocio + OWNER + suscripción atómicos; demo vencida → no visible; auditoría |
| T4.4 ✅ | Gestión del negocio (datos, ubicación, políticas, avisos) | OWNER sí, MANAGER 403 en lo exclusivo, otro negocio 404 |
| T4.5 ✅ | Miembros: listar, agregar por email, cambiar rol, revocar | Solo OWNER (el responsable; puede haber varios); siempre al menos un OWNER; efecto inmediato |
| T4.6 ✅ | Búsqueda pública por ciudad y texto, detalle por slug | Solo activos; sin datos internos |
| T4.7 ✅ | Suite cross-tenant/IDOR, contrato, E2E en QA, cierre de fase | Suite verde; cierre aprobado |

## Fase 5 — Profesionales y servicios (tareas)

Desglose aprobado por el usuario el 2026-11-24. Actores: administrador de la plataforma,
responsable del negocio (OWNER, puede ser también barbero), barbero/estilista (PROFESSIONAL) y
cliente. Todo barbero tiene cuenta propia. El responsable gestiona todo; el barbero agenda citas a
clientes (Fase 6), ajusta sus propios descansos y registra sus propias ausencias. Cada servicio
tiene una sola duración y un solo precio, iguales para todos los barberos; los combos ("corte +
barba") son servicios normales con su propio precio y duración.

| ID | Tarea | Criterio de aceptación principal |
|---|---|---|
| T5.1 ✅ | Quitar el rol MANAGER: solo OWNER (responsable) y PROFESSIONAL (barbero/estilista) | MANAGER rechazado por API y esquema; lo que hacía lo hace solo el OWNER |
| T5.2 ✅ | Profesionales: perfil ligado a la cuenta de un miembro (barbero o responsable que atiende), nombre visible, descripción, activo/inactivo, orden | Solo OWNER gestiona; otro negocio 404; auditoría |
| T5.3 ✅ | Horario semanal (franjas por día) y descansos recurrentes: OWNER para todos, el barbero para el suyo | Sin solapes; descanso dentro de una franja; zona horaria del negocio |
| T5.4 ✅ | Ausencias e incapacidades (OWNER para todos, el barbero las suyas) | Sin solapes por profesional (exclusion constraint) |
| T5.5 ✅ | Catálogo: categorías opcionales y servicios (nombre, descripción, duración, precio en COP) | Moneda del negocio, unidades menores, nombre único por negocio |
| T5.6 ✅ | Servicios que presta cada profesional | Mismo negocio; solo OWNER |
| T5.7 ✅ | Vista pública de profesionales y servicios; la búsqueda exige al menos un servicio y un profesional activos | Sin datos internos |
| T5.8 ✅ | Suite de aislamiento, E2E en QA, cierre de fase | Aprobación del usuario |

## Fase 6 — Disponibilidad y reservas (tareas)

Desglose aprobado por el usuario el 2026-11-24. Base: `availability-engine.md`,
`booking-lifecycle.md`, ADR-0004 y ADR-0005. Las notificaciones (WhatsApp, push) son de la Fase 7;
aquí cada cambio de una cita queda registrado en `appointment_events` para que la Fase 7 lo envíe.
Decisiones: el cliente elige barbero (obligatorio); toda cuenta se crea con email verificado y
celular obligatorio (desde T6.4); las ausencias hacen de bloqueos; una ausencia o cambio de horario que choca
con citas activas se rechaza con la lista de conflictos; un barbero se puede retirar o desactivar
aunque tenga citas futuras, que quedan por reasignar y el responsable las pasa a otro barbero.

| ID | Tarea | Criterio de aceptación principal |
|---|---|---|
| T6.1 ✅ | Motor de disponibilidad (función pura del dominio, módulo `scheduling`) | Tabla de casos, DST en varias zonas, igual a una referencia minuto a minuto |
| T6.2 ✅ | Disponibilidad pública por barbero, servicio y fecha | Solo barberos reservables que prestan el servicio; hora local y UTC; sin datos internos |
| T6.3 ✅ | Reserva del cliente (cuenta con email verificado y teléfono), snapshot del servicio, `Idempotency-Key` | Cero doble reserva: lock por profesional + exclusion constraint; 50 en paralelo → 1 éxito |
| T6.4 ✅ | Clientes del negocio y citas manuales (responsable; barbero solo en su agenda), pueden nacer confirmadas | Mismas garantías; barbero en agenda ajena → 403 |
| T6.5 ✅ | Agenda del negocio y del barbero (día, semana, mes, año) y "mis citas" del cliente (todos los negocios) | Barbero solo la suya; responsable todas; cliente solo las suyas; otro negocio → 404 |
| T6.6 ✅ | Transiciones: confirmar, cancelar, reprogramar (límites del negocio), iniciar, completar, no asistió; historial | Transición inválida → 409; idempotentes; concurrencia coherente (`version`) |
| T6.7 ✅ | Coherencia de agenda: ausencias y horario toman el lock y se rechazan si chocan con citas activas; retiro/desactivación de barbero deja sus citas por reasignar y el responsable las reasigna | Reserva vs ausencia en paralelo: solo una gana |
| T6.8 ✅ | Suite de aislamiento, las 7 pruebas de concurrencia del diseño, E2E en QA, cierre | Aprobación del usuario |

## Fase 7 — WhatsApp y push (tareas)

Desglose aprobado por el usuario el 2026-11-24. Base: `notifications.md`, ADR-0008, ADR-0009 y
ADR-0010. Mientras Meta verifica el negocio (P17: el usuario inicia el trámite) se desarrolla con el
número de prueba de Meta; pasar a clientes reales es configuración, no código. Decisiones del
usuario: agendar una cita es la aceptación del cliente a recibir WhatsApp sobre ella (también
cuando el barbero lo agenda a mano); el enlace de descarga de la app es configurable (P40); push se
construye ahora y se valida con la app real cuando exista.

**Replanificación del 2026-11-25 (decisión del usuario, ADR-0019):** la push se aplaza a la fase de
las apps móviles; en su lugar, correo con la marca **Booka** al cliente y al barbero, con acciones
(confirmar, cancelar, reagendar) en **dos toques**; los canales los configura cada persona (el
cliente conserva al menos uno); límite de reprogramaciones por negocio.

| ID | Tarea | Criterio de aceptación principal |
|---|---|---|
| T7.1 ✅ | Base: módulo `notifications`, mensajes encolados en la transacción del cambio, envío por River con proveedor simulado, revalidación de la cita, interruptor y lista blanca | Un mensaje por cambio; nunca dos veces; si el cambio se deshace, el mensaje también |
| T7.2 ✅ | Cliente de WhatsApp Cloud API (número de prueba), plantillas versionadas en código, reintentos y errores permanentes | Mensaje real al celular de prueba en QA |
| T7.3 ✅ | Mensajes al cliente: cita creada, cancelada por el negocio, reprogramada; P40: enlace de la app en la cita manual | Textos aprobados por el usuario |
| T7.4 ✅ | Recordatorios según el negocio, con botones Confirmar / Cancelar | Un recordatorio viejo se descarta |
| T7.5 ✅ | Webhook de Meta: firma, duplicados, estados de entrega y respuestas de los botones (token de un solo uso) | Firma falsa → rechazo; botón reusado → sin efecto |
| T7.6 | Correos de citas (Booka): HTML + texto, al cliente (los mismos avisos que WhatsApp, además del WhatsApp) y al barbero (reservas, cancelaciones y cambios); datos, política del negocio, dirección con mapa, contacto | Correo con diseño aprobado por el usuario, visto en Mailpit QA |
| T7.7 | Acciones desde el correo en dos toques: confirmar y cancelar (página con botón, código de un solo uso); reagendar: servicios para el frontend, con el código | Abrir el enlace no cambia nada; código reusado → sin efecto |
| T7.8 | Límite de reprogramaciones por cita, configurable por negocio | Superado el límite → rechazo, para cliente y botones |
| T7.9 | Preferencias de notificación: el cliente elige WhatsApp y/o correo (al menos uno) y apaga los recordatorios; el barbero activa o apaga sus correos. Control de costos (P16) | Sin canal → rechazo; tope alcanzado → no se envía |
| T7.10 | E2E en QA con mensajes reales, pruebas de seguridad del webhook y de los enlaces, cierre | Aprobación del usuario |
| — | Push (Expo, ADR-0010): **aplazada** a la fase de las apps móviles | — |

Las fases 8–13 se desglosarán en tareas al iniciar cada una.

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
| R8 | Alcance del MVP crece (pagos, reseñas…) | Alta | Medio | Roadmap estricto; pagos aceptados en la Fase 11B (ADR-0018); reseñas y otros fuera del MVP |
| R9 | Notificaciones obsoletas (recordatorio de cita cancelada) | Media | Medio | Verificación de versión al ejecutar job + cancelación de jobs |
| R10 | Cumplimiento legal de datos personales por país | Media | Alto | Consentimiento registrado, minimización, definir países del piloto |
| R11 | Entorno local Windows sin Go/herramientas | Alta | Bajo | T1.1 instala toolchain; todo reproducible también en Docker/CI |
| R12 | Deriva entre OpenAPI y la implementación | Media | Medio | Validación de respuestas contra el spec en pruebas |
