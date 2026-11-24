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
| 2026-11-19 | Autorización dentro del caso de uso (`contract.Authorizer`), no como middleware de ruta | ADR-0007: políticas explícitas en la capa de aplicación; el caso de uso conoce el recurso | Aplicado (T2.6) |
| 2026-11-19 | `PLATFORM_ADMIN` sin rol implícito en negocios | Mínimo privilegio; las acciones de soporte serán casos de uso de administración auditados | ⚠️ Reemplazada 2026-11-24 (ADR-0018) |
| 2026-11-19 | Sentinelas `authz.ErrForbidden`/`ErrNotFound` en `internal/platform/authz` y mapeo único `httpx.WriteAuthzProblem` | Todos los módulos responden 403/404 igual sin importarse entre sí | Aplicado (T2.6) |
| 2026-11-19 | Rol desconocido leído de la BD → error (falla cerrado), no se ignora | Defensa ante deriva entre esquema y código | Aplicado (T2.6) |
| 2026-11-19 | Rate limiting propio en memoria (token bucket + backoff), sin dependencias nuevas | Una instancia en el MVP; código pequeño y probado con reloj inyectado | Aplicado (T2.7) |
| 2026-11-19 | Clave de cliente: IPv4 completa o prefijo IPv6 /64 | Un usuario controla un /64 completo; por dirección sería trivial rotar | Aplicado (T2.7) |
| 2026-11-19 | Tablas acotadas: al llenarse se limpian inactivas y se expulsa una entrada aleatoria | Evitar DoS de memoria sin bloquear a usuarios nuevos | Aplicado (T2.7) |
| 2026-11-19 | Backoff de login por hash(email)+IP, aplicado exista o no la cuenta, antes de Argon2 | Anti-enumeración y sin costo de CPU para el atacante bloqueado; sin bloqueo de cuenta (evita DoS a la víctima) | Aplicado (T2.7) |
| 2026-11-19 | Límite estricto compartido entre todos los endpoints de credenciales y email | Un único presupuesto por cliente impide repartir el abuso entre rutas | Aplicado (T2.7) |
| 2026-11-19 | Auditoría transaccional para eventos con cambio de datos; *best-effort* en transacción propia para fallos y denegaciones | Ningún cambio sin rastro, sin convertir la tabla de auditoría en dependencia de disponibilidad del login | Aplicado (T2.8) |
| 2026-11-19 | `platform/reqctx` transporta request ID e IP del cliente hasta la capa de aplicación | Auditar sin acoplar los casos de uso a HTTP ni cambiar todas las firmas | Aplicado (T2.8) |
| 2026-11-19 | Accesos permitidos no se auditan; solo denegaciones (`authz.denied`) | Volumen: cada petición autorizada generaría una fila | Aplicado (T2.8) |
| 2026-11-19 | `contract.Authorizer.RequirePlatformAdmin` recibe `ctx` | Necesario para auditar la denegación; sin consumidores aún | Aplicado (T2.8) |
| 2026-11-19 | `NewService` usa reloj del sistema y logger descartado si no se inyectan | Evitar pánicos en wiring parcial (pruebas, herramientas) | Aplicado (T2.8) |
| 2026-11-19 | Contraseña mínima de 8 caracteres y lista pequeña de comunes (antes 10) | Decisión del usuario (P31): mínimo NIST; excepción frente a ASVS L2 documentada; se propuso 7 y se acordó 8 tras advertir que 7 queda bajo NIST | Aplicado (T2.10) |
| 2026-11-19 | MFA para administradores y gestión de sesiones individuales en la Fase 12 | Decisión del usuario (P32, P33) | Pendiente (Fase 12) |
| 2026-11-19 | Cierre de la Fase 2 | Aprobado por el usuario | Aplicado |
| 2026-11-22 | Droplet QA creado por el usuario (`nyc3`, Ubuntu 24.04); renombrado a `booking-qa` | Autorización del usuario | Aplicado |
| 2026-11-22 | Bootstrap en dos pasos: preparar y verificar `deploy`, luego `--lock-root` | Nunca dejar al operador sin acceso | Aplicado (T3.1) |
| 2026-11-22 | Usuario único `deploy` (llave, sudo sin contraseña, grupo docker) | Simplicidad MVP; sin contraseñas que robar; llaves de CI se restringirán en T3.5 | Aplicado (T3.1) |
| 2026-11-22 | Reinicio automático por actualizaciones de seguridad a las 04:30 UTC en QA | Parches sin intervención; impacto aceptable en QA | Aplicado (T3.1) |
| 2026-11-22 | shellcheck en CI (imagen fijada por digest) y `task shellcheck` | Scripts de despliegue revisados como el código | Aplicado (T3.1) |
| 2026-11-22 | QA con 1 GB + swap; subir a 2 GB solo si las mediciones lo exigen | Decisión del usuario (P34) | Aplicado |
| 2026-11-22 | DNS de QA: DuckDNS `booking-platform-col.duckdns.org` | Decisión del usuario (P35); gratuito | Aplicado |
| 2026-11-22 | Sin Cloud Firewall de DO en QA: `ufw` + solo Caddy expuesto | Decisión del usuario (P36) | Aplicado |
| 2026-11-22 | Roles `app_migrator` (dueño, DDL) y `app_runtime` (DML vía privilegios por defecto) creados en el initdb | Mínimo privilegio; la API comprometida no puede alterar el esquema | Aplicado (T3.2) |
| 2026-11-22 | Migraciones en un servicio `migrate` aparte; api/worker nunca reciben la contraseña del migrador | Separación de credenciales | Aplicado (T3.2) |
| 2026-11-22 | `sslmode=disable` dentro de la red interna de Docker (mismo host) | Tráfico que no sale del host; TLS a la BD se reevalúa con BD gestionada (Fase 13) | Aplicado (T3.2) |
| 2026-11-23 | Fase 4: piloto Colombia; solo el administrador crea negocios; miembros por email (decisiones del usuario) | P5, P10 | Aplicado (desglose) |
| 2026-11-23 | `audit_log.business_id` sin FK a `businesses`; `memberships.business_id` con FK `RESTRICT` (T4.2) | Las denegaciones auditan IDs de negocios inexistentes (sondeos IDOR): una FK haría perder ese rastro | Aplicado (T4.2, migración 00006) |
| 2026-11-24 | Búsqueda pública: solo `ACTIVE` + suscripción habilitada; el filtro de suscripción se pide en lote al contract de subscriptions (`EntitledAmong`), sin consultar tablas de otro módulo; hasta 5 lotes por página (una página puede traer menos que `limit` y seguir con `nextCursor`) | Respeta las fronteras de módulos sin N+1 | Aplicado (T4.6) |
| 2026-11-24 | Orden por `lower(name)` con la intercalación de la base (ñ junto a n) y cursor opaco con la clave que calcula PostgreSQL; texto con comodines `%`/`_` escapados (búsqueda literal), 2–60 caracteres; `cityId` obligatorio | Paginación estable y sin inyección de patrones | Aplicado (T4.6, migración 00009 con índice parcial) |
| 2026-11-24 | Vista pública sin datos internos: sin estado, recordatorios, intervalos ni fechas; con las reglas que el cliente necesita (antelación, horizonte, límites de cancelación y reprogramación) | Mínima exposición | Aplicado (T4.6) |
| 2026-11-24 | El responsable del negocio es el rol OWNER y **puede haber varios**; siempre al menos uno; el responsable agrega responsables, gerentes y barberos; cambio de dueño = asignar nuevo responsable (decisión del usuario) | Modelo de operación del usuario | Aplicado (T4.5, migración 00008 elimina el índice de OWNER único) |
| 2026-11-24 | "Al menos un OWNER" se garantiza bloqueando las filas OWNER del negocio (`SELECT … FOR UPDATE`) antes de quitar o degradar | Dos dueños que se retiran mutuamente a la vez no dejan el negocio sin responsable (verificado con concurrencia real) | Aplicado (T4.5) |
| 2026-11-24 | Re-agregar a un miembro retirado reactiva su fila (upsert) con el nuevo rol | La unicidad usuario-negocio se mantiene y el historial queda en `audit_log` | Aplicado (T4.5) |
| 2026-11-24 | Endpoints de miembros en el módulo identity (`/business/{id}/members`), que es dueño de las membresías | Sin que otro módulo toque la tabla | Aplicado (T4.5) |
| 2026-11-24 | Edición del negocio: cambio de ciudad solo dentro del mismo país (la moneda es del país, `CITY_OTHER_COUNTRY`) y la zona horaria sigue a la ciudad; slug, estado y moneda no editables por el dueño | Coherencia de precios y horarios; el slug es una URL pública estable | Aplicado (T4.4) |
| 2026-11-24 | PATCH con semántica "solo lo presente": `coordinates: null` elimina; políticas y recordatorios parciales; sin cambios = no-op sin auditoría | Clientes simples, auditoría con los campos realmente cambiados | Aplicado (T4.4) |
| 2026-11-24 | Una demo por cuenta: `identity.ClaimOwnership` bloquea la fila del usuario (`SELECT … FOR UPDATE`) y rechaza si ya es OWNER de un negocio activo | Dos solicitudes simultáneas no pueden crear dos demos (verificado con concurrencia real) | Aplicado (T4.3b) |
| 2026-11-24 | Todo negocio (autoservicio o creado por el admin) nace con suscripción `TRIALING` de 7 días; los negocios previos de QA reciben una demo nueva en la migración 00007 | Un único modelo; nada queda sin suscripción | Aplicado (T4.3b) |
| 2026-11-24 | `Entitled` es falso si la demo ya terminó aunque el job aún no la marque `EXPIRED`; job horario único por hora con `FOR UPDATE SKIP LOCKED` en lotes de 500 | Corte exacto sin depender de la hora del job; varios workers no se pisan | Aplicado (T4.3b) |
| 2026-11-24 | ADR-0018: alta autoservicio con demo de 7 días (con reservas reales), módulo `subscriptions` separado de la moderación, retención 30 días tras vencer, pagos en la Fase 11B (decisiones del usuario) | Modelo de negocio SaaS del usuario | Aceptado |
| 2026-11-24 | `PLATFORM_ADMIN` puede ejecutar cualquier caso de uso de cualquier negocio, auditado (decisión del usuario) | Soporte y operación de la plataforma | Aplicado (T4.4: `RequireBusinessRole` lo deja pasar con poderes de OWNER; cada caso de uso audita con el administrador como actor) |
| 2026-11-24 | MANAGER: ve la configuración del negocio pero no la edita (configuración = OWNER, security.md) | Propuesta sin objeción del usuario | Aplicado (T4.4) |
| 2026-11-24 | Un negocio nuevo nace `PENDING_REVIEW` (invisible) hasta que el administrador lo activa (decisión del usuario) | Se configura (profesionales, servicios en la Fase 5) antes de mostrarse a clientes | Aplicado (T4.3) |
| 2026-11-24 | Transacción compartida entre módulos por `context` (`db.ContextWithTx`; `db.WithTx` se une a la transacción del contexto) | Negocio + membresía OWNER (identity) + auditoría se confirman o revierten juntos sin que un módulo toque tablas de otro | Aplicado (T4.3) |
| 2026-11-24 | `identity/contract.MemberAdder.AddMember`: autoriza PLATFORM_ADMIN u OWNER del negocio, audita `membership.granted`; email malformado = cuenta no elegible | Única puerta para crear membresías (T4.3 y T4.5) | Aplicado (T4.3) |
| 2026-11-24 | Activar exige ciudad, división y país activos (409 `LOCATION_INACTIVE`); activar/suspender son idempotentes y solo se auditan si cambian | Un negocio activo debe ser alcanzable | Aplicado (T4.3) |
| 2026-11-24 | Listado admin con paginación por clave (`cursor` = último ID, UUIDv7) y filtros `status`/`cityId`, máx. 100 | Estable bajo inserciones, sin `OFFSET` | Aplicado (T4.3) |
| 2026-11-24 | Ubicación coherente por esquema: FK compuestas `(city_id, division_id)` → cities y `(division_id, country_code)` → divisions | La ciudad pertenece a la división y la división al país aunque falle la aplicación | Aplicado (T4.2) |
| 2026-11-24 | Coordenadas como `latitude`/`longitude` y `location geography(Point)` **generada** (GIST) | Las consultas SQLC no construyen valores PostGIS; búsqueda por cercanía lista para el futuro | Aplicado (T4.2) |
| 2026-11-24 | Slug: derivado del nombre sin tildes, 3–60 `[a-z0-9-]`, sin `--`, palabras reservadas (admin, api, login, soporte…), colisión → `-2`, `-3`…; `citext` único | URLs públicas estables y sin suplantar páginas de la plataforma | Aplicado (T4.2) |
| 2026-11-24 | Teléfono con `nyaruka/phonenumbers` (v1.8.1), guardado en E.164 con región por defecto del país | Validación real de números (móvil y fijo); la imagen crece ~8 MB por los metadatos | Aplicado (T4.2) |
| 2026-11-24 | No se puede desactivar país, división ni ciudad con negocios ACTIVE (409 `IN_USE`) | Extiende el invariante de ciudades a sus padres: ocultar un área no debe dejar negocios activos inalcanzables | Aplicado (T4.2) |
| 2026-11-24 | `geography/contract.Places.PlaceOfCity`: negocios recibe solo `cityId` y obtiene división, país, zona horaria y moneda | Una sola fuente de verdad para la ubicación; el cliente no puede enviar combinaciones incoherentes | Aplicado (T4.2) |
| 2026-11-23 | Paquete `internal/platform/audit` (constructor + escritura vía `platformq`) para los módulos nuevos; identity conserva el suyo | Auditoría en la misma transacción sin depender de identity; migrar identity después es opcional | Aplicado (T4.1) |
| 2026-11-23 | `PLATFORM_ADMIN` solo con el comando de operador `api admin grant-platform-admin EMAIL` (auditado, `source=cli`), nunca por HTTP | No existía forma de crear administradores sin SQL; el acceso al servidor es la barrera | Aplicado (T4.1) |
| 2026-11-23 | Geografía: nombres únicos sin distinguir mayúsculas por padre, código ISO 3166-2 opcional y del mismo país, tipos DEPARTMENT/STATE/PROVINCE/REGION/DISTRICT, monedas permitidas COP y USD | Evitar duplicados; agregar moneda es un cambio deliberado de código | Aplicado (T4.1) |
| 2026-11-23 | Lecturas públicas de geografía: solo registros activos; país o división inactivos ocultan lo que cuelga de ellos (404) | Desactivar es la forma de retirar datos sin borrarlos | Aplicado (T4.1) |
| 2026-11-23 | `release-tag.yml`: el tag `vX.Y.Z` re-etiqueta la imagen `qa-<sha>` validada (2.º padre del merge commit) sin reconstruir y verifica el digest | ADR-0017 (mismo artefacto en PROD); la retención de GHCR conserva las versiones con tag de release | Aplicado (T3.8); se usará en la Fase 13 |
| 2026-11-23 | `qa` → `main` solo al pasar a producción (decisión del usuario); no se crea `v0.3.0` | `main` = producción; no hay PROD hasta la Fase 13 | Aplicado (T3.8) |
| 2026-11-23 | Fase 3 aprobada por el usuario | `phase-3-closure.md` | Aplicado |
| 2026-11-23 | Argon2id QA: 64 MiB, t=2, p=2 (P27) | Medido en el Droplet: 222–262 ms (objetivo 100–250 ms); t=3 = 340 ms. Supera los mínimos de OWASP (19 MiB t=2) | Aplicado (T3.7) |
| 2026-11-23 | E2E de QA como script versionado (`scripts/e2e-qa.sh`) que respeta `Retry-After` | Repetible tras cada despliegue; el límite de 10/min en autenticación es intencional | Aplicado (T3.7) |
| 2026-11-23 | Backups QA con systemd timer + `pg_dump -Fc` en el mismo Droplet (7 diarios, 4 semanales), sin cifrado local | Sin costo (decisión del usuario); en el mismo disco el cifrado no añade protección; la copia externa queda como riesgo aceptado (P38) | Aplicado (T3.6) |
| 2026-11-23 | Prueba de restauración automática semanal en base temporal (`booking-backup verify`) | Un backup no probado no es un backup; detecta archivos truncados y cambios de esquema | Aplicado (T3.6) |
| 2026-11-23 | Restauración en sitio con `pg_restore --clean --create` y dump de seguridad previo | Conserva dueño, permisos y ACL de la base (roles de mínimo privilegio) | Aplicado (T3.6) |
| 2026-11-23 | Despliegue automático a QA en cada push a `qa` (decisión del usuario) | Ciclo corto; QA solo tiene datos sintéticos; rollback automático | Aplicado (T3.5) |
| 2026-11-23 | El stack de Compose viaja dentro de la imagen; el servidor solo acepta `deploy <imagen GHCR@digest>` | La llave de CI no puede enviar archivos ni comandos: aunque se filtre, solo despliega imágenes publicadas por este repositorio | Aplicado (T3.5) |
| 2026-11-23 | Llave SSH de CI con `restrict,command=` y host key fijada en secreto; token de GHCR = `GITHUB_TOKEN` efímero por stdin | Mínimo privilegio; sin credenciales de registro persistentes en el servidor | Aplicado (T3.5) |
| 2026-11-23 | Se publica la imagen escaneada (sin reconstruir); SBOM CycloneDX con Trivy como artefacto, sin atestaciones buildx | Imagen de un solo manifiesto: la retención en GHCR borra versiones completas; las atestaciones de GitHub no están disponibles en repos privados del plan Free | Aplicado (T3.4) |
| 2026-11-23 | GHCR privado, conservar las 10 últimas `qa-*` (decisión del usuario) | Plan Free: 500 MB | Aplicado (T3.4) |
| 2026-11-23 | Versión de la imagen = commit corto; tag `qa-<sha>` | El smoke test compara `/healthz` con el commit; PROD promoverá el mismo digest | Aplicado (T3.4) |
| 2026-11-23 | Backup `pg_dump` antes de cada despliegue, 5 en el servidor | Restaurar si una migración daña datos; backups diarios en T3.6 | Aplicado (T3.5) |
| 2026-11-22 | Caddy 2.11.9 como único punto de entrada; IP fija en la red interna y `TRUSTED_PROXIES` con solo esa IP | La API no puede ser engañada por `X-Forwarded-For` de clientes ni de otros contenedores | Aplicado (T3.3) |
| 2026-11-22 | HSTS 1 año sin `includeSubDomains`/`preload` | Dominio en zona DNS compartida gratuita (DuckDNS) | Aplicado (T3.3) |
| 2026-11-22 | Sin access log en Caddy | La API ya registra cada petición con redacción; evitar duplicar datos personales | Aplicado (T3.3) |
| 2026-11-22 | Reintentos (`lb_try_duration 10s`) en lugar de health check activo | Con un único upstream, el health check activo devolvía 503 durante el arranque; los reintentos cubren reinicios en despliegues | Aplicado (T3.3) |
| 2026-11-22 | Hasta T3.4 la imagen se transfiere por SSH (`docker save \| docker load`) | Sin credenciales de registro en el servidor todavía | Temporal (T3.4) |
| 2026-11-18 | Imágenes en GHCR | Gratuito con GitHub, integrado con Actions | Propuesto |
| 2026-11-18 | Búsqueda de negocios por ciudad en MVP (PostGIS listo para cercanía futura) | Simplicidad | Propuesto |
| 2026-11-18 | Sin auto-cancelación de citas pendientes en MVP | Regla del prompt; requiere política aprobada | Propuesto |
| 2026-11-18 | Recurso de otro negocio → 404 | Evita enumeración (IDOR) | Aplicado (T2.6) |
