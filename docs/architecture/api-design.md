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

### Alta de negocios (PLATFORM_ADMIN, T4.3)
`POST /admin/businesses` (con `ownerEmail`; nace `PENDING_REVIEW`), `GET /admin/businesses`
(`status`, `cityId`, `cursor`, `limit`), `GET /admin/businesses/{id}`,
`POST /admin/businesses/{id}/activate`, `POST /admin/businesses/{id}/suspend`.

### Alta autoservicio y suscripciones (ADR-0018, T4.3b)
`POST /businesses` (cuenta verificada → OWNER de un negocio `ACTIVE` con demo de 7 días; una
por cuenta, 409 `ALREADY_OWNER`; límite estricto por cliente).
`GET /admin/businesses/{id}/subscription`, `POST /admin/businesses/{id}/subscription/extend-trial`
(`days` 1–90), `POST /admin/businesses/{id}/subscription/activate` (cortesía hasta la Fase 11B).

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
