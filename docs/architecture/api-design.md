# Diseño de API

Estado: **Propuesta — Fase 0.** ADR-0011.

## 1. Convenciones

- REST sobre HTTPS, JSON, prefijo `/api/v1`.
- Contrato **OpenAPI 3.1** en `api/openapi.yaml` como fuente de verdad (spec-first). Los
  frontends generan sus clientes TypeScript desde él.
- Router: `net/http` de la stdlib (patrones con método y comodines). Sin framework.
- Pruebas de contrato: las respuestas de los tests de integración se validan contra el spec.
- Errores RFC 9457 (`application/problem+json`) con `type`, `title`, `status`, `detail`,
  `code` (estable, para el frontend: `SLOT_UNAVAILABLE`, `INVALID_TRANSITION`, …) y `errors[]`
  por campo en validaciones.
- Fechas ISO 8601 con offset; instantes devueltos en UTC (`Z`) más campo local cuando aplica.
- Dinero: `{ "amountMinor": 2500000, "currency": "COP" }`.
- Paginación por cursor: `?limit=&cursor=` → `{ items, nextCursor }`.
- `Idempotency-Key` obligatorio en `POST /appointments` y recomendado en otros POST críticos.
- `If-Match`/`version` opcional en transiciones de cita para detectar ediciones concurrentes.
- Health: `GET /healthz` (vivo), `GET /readyz` (BD accesible, migraciones al día) — sin auth, sin
  datos internos.

## 2. Endpoints principales (MVP)

### Identidad
| Método | Ruta | Actor |
|---|---|---|
| POST | `/auth/register` | Público |
| POST | `/auth/login` | Público |
| POST | `/auth/logout` | Autenticado |
| POST | `/auth/logout-all` | Autenticado |
| POST | `/auth/password/forgot` · `/auth/password/reset` | Público |
| GET/PATCH | `/me` | Autenticado |
| GET | `/me/sessions` · DELETE `/me/sessions/{id}` | Autenticado |
| POST/DELETE | `/me/devices` · `/me/devices/{id}` | Autenticado |

### Geografía (lectura pública, escritura admin)
`GET /countries`, `GET /countries/{code}/divisions`, `GET /divisions/{id}/cities`;
`POST /admin/countries|divisions|cities`, `PATCH /admin/countries/{code}`,
`PATCH /admin/divisions/{id}`, `PATCH /admin/cities/{id}` (implementado en T4.1; detalle en
`api/openapi.yaml`).

### Alta de negocios por el administrador (opción de soporte, T4.3)
`POST /admin/businesses` (con `ownerEmail`; nace `ACTIVE` con demo, igual que el autoservicio), `GET /admin/businesses`
(`status`, `cityId`, `cursor`, `limit`), `GET /admin/businesses/{id}`,
`POST /admin/businesses/{id}/activate`, `POST /admin/businesses/{id}/suspend`.

### Alta autoservicio y suscripciones (ADR-0018, T4.3b)
`POST /businesses` (cuenta verificada → OWNER de un negocio `ACTIVE` con demo de 7 días; hasta
`OWNED_BUSINESS_LIMIT` negocios propios por cuenta, 2 durante la demo, luego 409
`OWNED_BUSINESS_LIMIT` (P-11); `Idempotency-Key` opcional; límite estricto por cliente).
### Imágenes (2026-11-27, ADR-0020)
`PUT|DELETE /business/{id}/logo` y `/cover` (OWNER) y
`GET|POST /business/{id}/professionals/{pid}/portfolio`, `PATCH|DELETE …/portfolio/{imageId}`
(el profesional el suyo, el OWNER cualquiera; 10 como máximo, luego 409 `PORTFOLIO_FULL`). Subidas
en `multipart/form-data` (campo `image`, 5 MiB; `caption` opcional en el portafolio). Errores de
imagen: 400 con `errors[].field = image` (`REQUIRED`, `UNSUPPORTED_IMAGE`, `INVALID_IMAGE`,
`IMAGE_TOO_BIG`), 413 y 415. `Business` y `PublicBusiness` traen `logoUrl` y `coverUrl` (null sin
imagen) y el catálogo público trae `portfolio` en cada profesional. Las imágenes se sirven en
`GET /media/{key}` (público, inmutable).

### Estadísticas (P-12, 2026-11-27)
`GET /business/{id}/stats?from=&to=&groupBy=&professionalId=` (OWNER todo el negocio o un
profesional, con `byProfessional`; PROFESSIONAL solo lo suyo, otro → 403) y
`GET /me/stats?from=&to=&groupBy=` (negocios donde la cuenta es OWNER, con `byBusiness`).
Rango inclusivo en el calendario de cada negocio: hasta 366 días por día y 5 años por mes o año;
si no, 400 `INVALID_RANGE`. Las reglas de cada cifra están en `StatsFigures` (OpenAPI) y en
`scheduling/domain/stats.go`; los minutos disponibles usan el horario semanal actual.

`GET /admin/businesses/{id}/subscription`, `POST /admin/businesses/{id}/subscription/extend-trial`
(`days` 1–90), `POST /admin/businesses/{id}/subscription/activate` (cortesía hasta la Fase 11B).

### Gestión del negocio (T4.4)
`GET /business/{businessId}` (OWNER, PROFESSIONAL, PLATFORM_ADMIN): negocio + resumen de su
suscripción. `PATCH /business/{businessId}` (OWNER, PLATFORM_ADMIN; PROFESSIONAL 403): datos,
ubicación, política de reservas y recordatorios. Otro negocio → 404 idéntico a inexistente.

### Miembros (T4.5)
`GET /business/{businessId}/members` (OWNER), `POST …/members` (OWNER: email + rol OWNER o
PROFESSIONAL),
`PATCH …/members/{userId}` (rol) y `DELETE …/members/{userId}` (OWNER); 409 `LAST_OWNER`,
`ALREADY_MEMBER`; 400 `ACCOUNT_NOT_ELIGIBLE`.

### Profesionales (T5.2)
`GET /business/{businessId}/professionals` y `GET …/{professionalId}` (OWNER, PROFESSIONAL),
`POST …/professionals` (OWNER: email + nombre visible; agrega la membresía si hace falta) y
`PATCH …/{professionalId}` (OWNER). 409 `ALREADY_PROFESSIONAL`.

### Horario semanal (T5.3)
`GET …/professionals/{professionalId}/schedule` (OWNER, PROFESSIONAL) devuelve siempre los 7 días
(lunes primero) con `workingHours` y `breaks` (`HH:MM`, hora local del negocio).
`PUT …/schedule` reemplaza la semana completa (días omitidos = descanso): OWNER para cualquiera,
PROFESSIONAL solo el suyo (403). Errores por campo con ruta (`days[0].breaks[1]`) y códigos
`INVALID_WEEKDAY`, `DUPLICATE_WEEKDAY`, `INVALID_TIME`, `INVALID_RANGE`, `TOO_MANY_RANGES`,
`OVERLAPPING_RANGES`, `BREAK_OUTSIDE_WORKING_HOURS`.

### Ausencias (T5.4)
`GET …/professionals/{professionalId}/absences?from=&to=` (OWNER, PROFESSIONAL; por defecto desde
ahora y 2 años; máx. 200), `POST …/absences`, `PATCH …/absences/{absenceId}` y `DELETE …` (OWNER
para cualquiera, PROFESSIONAL solo las suyas, 403). `start`/`end` en RFC 3339 con zona; se
devuelven en UTC. 409 `ABSENCE_OVERLAP`; campos `INVALID_PERIOD`, `PERIOD_TOO_LONG`,
`INVALID_REASON`, `INVALID_WINDOW`.

### Catálogo (T5.5)
`GET/POST /business/{businessId}/service-categories`, `PATCH/DELETE …/{categoryId}`;
`GET/POST /business/{businessId}/services`, `GET/PATCH …/services/{serviceId}`. Lectura: OWNER y
PROFESSIONAL; escritura: OWNER (403). Dinero como `priceMinor` (unidades menores, múltiplo de 100:
sin centavos) + `currency` del negocio. `categoryId: null` en PATCH quita la categoría. 409
`CATEGORY_NAME_TAKEN` / `SERVICE_NAME_TAKEN`; campos `INVALID_DURATION`, `INVALID_PRICE`,
`CATEGORY_NOT_FOUND`, `INVALID_ID`.

### Servicios por profesional (T5.6)
`GET …/professionals/{professionalId}/services` (OWNER, PROFESSIONAL) y `PUT` con
`{"serviceIds": [...]}` (OWNER) que reemplaza la lista completa; `SERVICE_NOT_FOUND` en
`serviceIds[i]` si el servicio no es del negocio; profesional inexistente o ajeno → 404.

### Coherencia de la agenda (T6.7)
Ausencias (`POST|PATCH …/professionals/{id}/absences…`) y horario (`PUT …/schedule`) que
romperían citas activas → 409 `AGENDA_CONFLICTS` con `conflicts: [{id, startAt, endAt,
customerName, status}]`. `GET /api/v1/business/{businessId}/appointments/to-reassign` (OWNER):
citas activas de profesionales que ya no son reservables; cada ítem de la agenda trae
`needsReassignment`.

### Cambios de una cita (T6.6)
Cliente: `POST /api/v1/me/appointments/{id}/confirm | cancel | reschedule` (cancel: `reason`
opcional; reschedule: `startAt`). Personal: `POST /api/v1/business/{businessId}/appointments/{id}/
confirm | cancel | reschedule | start | complete | no-show` (cancel: `reason` obligatorio;
reschedule: `startAt` y, solo OWNER, `professionalId`) y `GET …/{id}/history`. Respuestas: la cita
con `version` y datos de cancelación. 409: `INVALID_TRANSITION`, `TOO_EARLY`,
`APPOINTMENT_STARTED`, `CUTOFF_PASSED`, `APPOINTMENT_CHANGED`, `SLOT_UNAVAILABLE`.

### Agenda y mis citas (T6.5)
`GET /api/v1/business/{businessId}/agenda?view=day|week|month|year&date=&professionalId=&cursor=&limit=`
(fecha por defecto: hoy en el negocio): `from`/`to` (días locales, inclusivos), `timeZone`,
`items` (hora UTC y local, barbero, servicio, precio, estado, cliente con nombre y celular) y
`nextCursor`. `GET /api/v1/me/appointments?scope=upcoming|past` (todas las del cliente, con
negocio y barbero) y `GET /api/v1/me/appointments/{appointmentId}`.

### Clientes del negocio y citas manuales (T6.4)
`GET|POST /api/v1/business/{businessId}/customers` (`?q=` por nombre o celular) y
`PATCH …/customers/{customerId}`: nombre y celular (E.164, único por negocio → 409
`CUSTOMER_PHONE_TAKEN`); `hasAccount` indica si reservó desde la app.
`POST /api/v1/business/{businessId}/appointments` con `Idempotency-Key`: `professionalId`,
`serviceId`, `startAt`, `confirmed` (por defecto `true`) y el cliente: `customer`
(`fullName`, `phone`, `email` opcional; la forma rápida, se reutiliza por celular o se crea) o
`customerId` (uno ya existente). Los clientes tienen `email` opcional. El registro
(`POST /auth/register`) exige `phone` desde T6.4.

### Reserva del cliente (T6.3)
`PUT /api/v1/me/phone` (`{"phone":"300 123 4567"}` → `+573001234567`) y
`POST /api/v1/appointments` con cabecera `Idempotency-Key` y cuerpo `businessId`, `serviceId`,
`professionalId`, `startAt` (un `startAt` de la disponibilidad). 201 con la cita
(`PENDING_CONFIRMATION`, snapshot del servicio); repetir con la misma clave devuelve la misma
cita. 404 único; 400 `DATE_OUTSIDE_BOOKING_WINDOW`; 409 `SLOT_UNAVAILABLE`; 422 `PHONE_REQUIRED`
/ `IDEMPOTENCY_KEY_REUSED`; 503 `AGENDA_BUSY` (reintentar tras `Retry-After`).

### Disponibilidad pública (T6.2)
`GET /api/v1/businesses/{slug}/availability?serviceId=&professionalId=&date=YYYY-MM-DD` (sin
autenticación, límite general): `date`, `timeZone`, `serviceId`, `professionalId`,
`durationMinutes` y `slots[]` con `startAt`/`endAt` (UTC) y `startLocal` (con el offset del
negocio). Informativa: la reserva revalida. 404 idéntico para negocio no público, servicio no
reservable con ese barbero o barbero no reservable; 400 `DATE_OUTSIDE_BOOKING_WINDOW`.
Colección de Postman para validarla en QA: `docs/testing/postman/`.

### Catálogo público (T5.7)
`GET /api/v1/businesses/{slug}/catalog` (sin autenticación, límite general): `categories`
(id, nombre), `services` (id, categoryId, nombre, descripción, duración, `priceMinor`,
`currency`, `professionalIds`) y `professionals` (id, `displayName`, `bio`, `serviceIds`).
Solo servicios activos prestados por profesionales reservables; sin cuentas, emails, estados
ni fechas. 404 con las mismas reglas que la ficha pública (indistinguible).

### Búsqueda pública (T4.6)
`GET /businesses?cityId=&q=&cursor=&limit=` y `GET /businesses/{slug}` (sin autenticación):
solo negocios `ACTIVE` con demo vigente o pagada; vista reducida sin datos internos.

### Descubrimiento (cliente)
| Método | Ruta |
|---|---|
| GET | `/businesses?cityId=&q=&cursor=` |
| GET | `/businesses/{slug}` |
| GET | `/businesses/{id}/services` |
| GET | `/businesses/{id}/professionals?serviceId=` |
| GET | `/businesses/{id}/availability?serviceId=&professionalId=&date=YYYY-MM-DD` |

### Citas del cliente
| Método | Ruta |
|---|---|
| POST | `/appointments` (Idempotency-Key) |
| GET | `/me/appointments?status=&cursor=` |
| GET | `/me/appointments/{id}` |
| POST | `/me/appointments/{id}/confirm` · `/cancel` · `/reschedule` |

### Gestión del negocio (OWNER / MANAGER; PROFESSIONAL limitado a su agenda)
Todas bajo `/business/{businessId}/…` — el `businessId` de la ruta se verifica contra las
membresías del principal en cada request.

| Recurso | Operaciones |
|---|---|
| `/` | GET, PATCH (datos, ubicación, políticas, notificaciones) |
| `/members` | GET, POST, PATCH, DELETE (OWNER) |
| `/professionals` | CRUD; `/professionals/{id}/schedule` PUT; `/breaks`; `/absences` |
| `/categories`, `/services` | CRUD; `/services/{id}/professionals` PUT |
| `/clients` | GET, POST, PATCH |
| `/agenda?professionalId=&from=&to=` | GET (día/semana/mes) |
| `/appointments` | POST (manual), GET |
| `/appointments/{id}/confirm·cancel·reschedule·start·complete·no-show` | POST |
| `/blocks` | GET, POST, DELETE |
| `/share-links` | POST, DELETE |

### Profesional
`GET /me/professional-profiles` (negocios donde trabaja) y luego las rutas
`/business/{businessId}/…` filtradas por su agenda.

### Enlaces compartidos
`GET /shared/{token}` — solo lectura, datos mínimos.

### Administración de plataforma
`/admin/users`, `/admin/businesses` (activar / suspender), `/admin/audit-log`, `/admin/settings`.

### Webhooks
`GET|POST /webhooks/whatsapp`.

## 3. Versionado

Cambios compatibles (campos nuevos opcionales) en `v1`. Cambios incompatibles → `v2` con
convivencia temporal. Las apps móviles envían `X-App-Version` para poder forzar actualización.
