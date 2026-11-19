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
| 2026-11-19 | ~~Una rama `feat/…` por tarea + PR a `main`; sin rama `qa`~~ | Reemplazado por ADR-0017 (ramas main/qa/feature/hotfix) | Reemplazado |
| 2026-11-19 | Estrategia de ramas main / qa / feature/* / hotfix/* | Decisión del usuario; ADR-0017 | Aceptado |
| 2026-11-19 | Nombres de rama `feature/…` (no `feat/…`) | ADR-0017; la rama remota `feat/t1.4-database` se renombra solo con autorización | Aceptado |
| 2026-11-19 | Dependabot: PRs contra `qa`; sin actualizaciones mayores automáticas de PostGIS/PostgreSQL | ADR-0017; un salto mayor de PG requiere migración de datos planificada | Aplicado (efectivo al llegar a `main`) |
| 2026-11-19 | Sin protección de ramas por ahora | Decisión del usuario (P23); se aplica por disciplina + CI | Aceptado |
| 2026-11-19 | Esquema de River como migración Goose `00002_river.sql` (River CLI v0.51.2, sin versión 1) | Un solo sistema de migraciones; al actualizar River se genera una migración nueva con las versiones nuevas | Aplicado |
| 2026-11-19 | El worker no arranca con migraciones pendientes | Evita errores en tablas inexistentes; el orquestador lo reinicia | Aplicado |
| 2026-11-19 | `omit_unused_structs` en sqlc | El esquema compartido no debe generar modelos ajenos en cada paquete | Aplicado |
| 2026-11-19 | Reintentos al arrancar contenedores de prueba | testcontainers en Windows falla intermitentemente al crear el provider | Aplicado |
| 2026-11-19 | Validación OpenAPI (spec + contrato) con pb33f/libopenapi en pruebas Go | Soporta 3.1; sin dependencia de imágenes externas | Aplicado |
| 2026-11-19 | 404/405 documentados como respuestas genéricas (no por operación) | Un 405 ocurre precisamente en métodos no definidos | Aplicado |
| 2026-11-19 | `task test:integration` fija `DOCKER_HOST` al endpoint del contexto Docker activo | Evita la detección intermitente de testcontainers en Windows | Aplicado |
| 2026-11-19 | Escáneres (gitleaks, Trivy) como imágenes Docker fijadas por digest en CI, no como acciones | Menor superficie de cadena de suministro | Aplicado |
| 2026-11-19 | Acciones de GitHub fijadas por SHA, versiones con ≥ 2 semanas de publicadas | Cadena de suministro | Aplicado |
| 2026-11-19 | Expiración de sesiones evaluada con el `now` de la aplicación en las consultas | Reloj inyectable y pruebas deterministas (ADR-0005) | Aplicado |
| 2026-11-19 | FK `business_id` de memberships/audit_log diferida a la Fase 4 | La tabla businesses aún no existe | Pendiente (Fase 4) |
| 2026-11-19 | Reglas depguard por módulo (frontera y dirección de dependencias) | Clean Architecture verificable en CI (ADR-0002) | Aplicado |
| 2026-11-19 | Argon2id 64 MiB / t=3 / p=2 por defecto; calibrar `ARGON2_ITERATIONS` en el Droplet QA | 71 ms en el equipo de desarrollo; el servidor será más lento; `needsRehash` permite subirlo sin migración | Aplicado (calibración pendiente Fase 3) |
| 2026-11-19 | Límites de costo al decodificar hashes y semáforo de concurrencia en Argon2 | Evitar DoS por hashes manipulados o logins masivos (64 MiB por hash) | Aplicado |
| 2026-11-19 | Verificación de email obligatoria antes del primer login | Decisión del usuario | Aplicado (T2.3) |
| 2026-11-19 | Registro y reenvío responden siempre igual (202) | Anti-enumeración de cuentas | Aplicado |
| 2026-11-19 | El token de verificación lo genera el worker al enviar; la cola solo lleva el user_id | Ningún token en claro en reposo (ni en river_job) | Aplicado |
| 2026-11-19 | Email vía SMTP genérico (stdlib); **Mailpit en DEV y QA** (ningún correo real sale de QA); TLS obligatorio solo en PROD | Decisión del usuario; independiente del proveedor (P13 abierto solo para PROD) | Aplicado |
| 2026-11-19 | La UI de Mailpit en QA no se expone públicamente (solo túnel SSH) | Muestra todos los correos, incluidos tokens de verificación | Pendiente (Fase 3) |
| 2026-11-19 | Endpoints JSON exigen `Content-Type: application/json` (415) | Bloquea POST de formularios entre sitios | Aplicado |
| 2026-11-19 | Dimensionar memoria ≥ ARGON2_MAX_CONCURRENT × ARGON2_MEMORY_KIB + margen | Evitar OOM por hashing concurrente | Aplicado |
| 2026-11-19 | Web: token solo en cookie `__Host-session` (HttpOnly, Secure, SameSite=Lax); móvil: Bearer en body | Token inaccesible a JS en web; apps nativas sin cookies | Aplicado |
| 2026-11-19 | CSRF: peticiones con cookie que cambian estado exigen `Origin` en la lista blanca | Defensa en profundidad sobre SameSite=Lax | Aplicado |
| 2026-11-19 | `EMAIL_NOT_VERIFIED` solo tras contraseña correcta; resto de fallos de login idénticos | No revelar cuentas ni su estado | Aplicado |
| 2026-11-19 | IP del cliente = par TCP; X-Forwarded-For se confiará solo desde Caddy (Fase 3) | Evitar suplantación de IP | Pendiente (Fase 3) |
| 2026-11-19 | Reset de contraseña: token `bp_pr_` 30 min, un solo uso; política validada antes de consumir el token; revoca todas las sesiones; aviso por email | security.md §1 | Aplicado |
| 2026-11-19 | Cambio autenticado de contraseña conserva la sesión actual y revoca las demás | Usabilidad sin perder seguridad | Aplicado |
| 2026-11-18 | Imágenes en GHCR | Gratuito con GitHub, integrado con Actions | Propuesto |
| 2026-11-18 | Búsqueda de negocios por ciudad en MVP (PostGIS listo para cercanía futura) | Simplicidad | Propuesto |
| 2026-11-18 | Sin auto-cancelación de citas pendientes en MVP | Regla del prompt; requiere política aprobada | Propuesto |
| 2026-11-18 | Recurso de otro negocio → 404 | Evita enumeración (IDOR) | Propuesto |
