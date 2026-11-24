# Modelo de datos (PostgreSQL)

Estado: **Propuesta — Fase 0.** El DDL definitivo se escribe como migraciones Goose en las fases
correspondientes. Lo siguiente es un **boceto de diseño**, no una migración.

## 1. Principios

- PostgreSQL (versión estable vigente al iniciar Fase 1; mínimo 16) + extensiones `postgis`,
  `btree_gist`, `citext`.
- PK `uuid` (UUIDv7 generado en la app) — ordenable por tiempo, sin enumeración.
- Fechas: `timestamptz` (siempre UTC en la app). Horarios semanales: `time` + `smallint` día.
- Dinero: `bigint` en unidades menores + `char(3)` moneda.
- Estados: `text` + `CHECK` (más fácil de migrar que `ENUM`).
- Toda tabla de datos de negocio tiene `business_id NOT NULL` con índice; las FK compuestas
  `(business_id, id)` garantizan que no se mezclen recursos de negocios distintos.
- `created_at`, `updated_at` en todas las tablas mutables; `version int` en agregados con
  concurrencia (citas).
- Un esquema por defecto (`public`); prefijo de tabla por módulo no es necesario. River usa sus
  propias tablas (`river_*`).

## 2. Tablas por módulo

### Identity & Access
```sql
users(id uuid pk, email citext unique not null, phone text, password_hash text not null,
      full_name text not null, status text not null, is_platform_admin bool not null default false,
      locale text not null default 'es', created_at, updated_at)
sessions(id uuid pk, user_id uuid fk, token_hash bytea unique not null, client_type text,
         created_at, last_seen_at, idle_expires_at timestamptz, absolute_expires_at timestamptz,
         revoked_at timestamptz, ip inet, user_agent text)
password_reset_tokens(token_hash bytea pk, user_id uuid fk, expires_at, used_at)
memberships(id uuid pk, user_id fk, business_id fk, role text, status text,
            unique(user_id, business_id))
  -- (T4.5, migración 00008) varios OWNER por negocio; "al menos uno" se garantiza en los casos de uso
audit_log(id uuid pk, occurred_at, actor_user_id, business_id null, action text,
          resource_type text, resource_id uuid, ip inet, outcome text, metadata jsonb)
```

### Geography
```sql
-- Implementado en 00005_geography.sql (T4.1): CHECK de formatos, nombres únicos por padre
-- (lower(name)), código ISO 3166-2 único y del mismo país, created_at/updated_at.
countries(code char(2) pk, name, phone_code, default_currency char(3), active)
administrative_divisions(id uuid pk, country_code fk, code text null, name, type, active)
cities(id uuid pk, division_id fk, name, time_zone text not null, active)
```

### Businesses
Implementado en `00006_businesses.sql` (T4.2): estados `PENDING_REVIEW`/`ACTIVE`/`SUSPENDED`,
`latitude`/`longitude` + `location` generada, FK compuestas de ubicación, CHECK de todos los
rangos de la política, recordatorios (`reminders_enabled`, `reminder_hours_before` 1–72), FK
`memberships.business_id → businesses` (RESTRICT). Diseño original:
```sql
businesses(id uuid pk, name, slug citext unique, status text, contact_phone, description,
           country_code fk, division_id fk, city_id fk, address_line text,
           location geography(Point, 4326) null, time_zone text not null, currency char(3),
           slot_interval_minutes smallint check (in (5,10,15,20)) default 10,
           buffer_minutes smallint check (between 0 and 60) default 0,
           min_lead_minutes int, max_advance_days int,
           cancellation_cutoff_hours int, reschedule_cutoff_hours int,
           require_customer_confirmation bool, reminder_hours_before int default 24,
           created_at, updated_at)
  -- GIST sobre location para búsquedas por cercanía (futuro); búsqueda MVP por city_id.
```

### Professionals
```sql
professionals(id uuid pk, business_id fk, user_id null, display_name, bio, status, sort_order,
              unique(business_id, id), unique(business_id, user_id))
professional_working_hours(id, business_id, professional_id, weekday smallint 0-6,
                           start_time time, end_time time, check (start_time < end_time))
professional_breaks(id, business_id, professional_id, weekday, start_time, end_time)
  -- T5.3 (00012): FK compuesta (business_id, professional_id) → professionals ON DELETE CASCADE;
  -- weekday 0 = domingo … 6 = sábado; solapes y contención validados por el dominio con la
  -- fila del profesional bloqueada (reemplazo de la semana completa en una transacción).
professional_absences(id, business_id, professional_id, period tstzrange not null, reason,
                      exclude using gist (professional_id with =, period with &&))
```

### Catalog
```sql
service_categories(id, business_id, name, sort_order, active, unique(business_id, name))
services(id, business_id, category_id, name, description, price_minor bigint check (>= 0),
         currency char(3), duration_minutes int check (between 5 and 480), kind text,
         status text, unique(business_id, name))
service_components(service_id, component_service_id)       -- combos, informativo
service_professionals(business_id, service_id, professional_id,
                      primary key(service_id, professional_id))
  -- FK compuestas (business_id, service_id) y (business_id, professional_id)
```

### Scheduling
```sql
business_clients(id, business_id, user_id null, full_name, phone null, email null, notes,
                 unique(business_id, user_id), unique(business_id, phone))
appointments(
  id uuid pk, business_id, professional_id, client_id, booked_by_user_id, channel text,
  service_id, service_name text, price_minor bigint, currency char(3), duration_minutes int,
  start_at timestamptz not null, end_at timestamptz not null, buffer_minutes smallint not null,
  occupied tstzrange generated always as
           (tstzrange(start_at, end_at + make_interval(mins => buffer_minutes), '[)')) stored,
  status text not null, version int not null default 1,
  cancelled_by text, cancellation_reason text, cancelled_at timestamptz,
  notes text, created_at, updated_at,
  check (end_at > start_at),
  constraint appointments_no_overlap exclude using gist (
     professional_id with =, occupied with &&
  ) where (status in ('PENDING_CONFIRMATION','CONFIRMED','IN_PROGRESS'))
)
appointment_events(id, appointment_id, business_id, type, from_status, to_status,
                   old_start_at, new_start_at, actor_user_id, actor_kind, occurred_at, metadata jsonb)
time_blocks(id, business_id, professional_id, period tstzrange, reason, created_by,
            exclude using gist (professional_id with =, period with &&))
agenda_share_links(id, business_id, professional_id, scope, resource_ref, token_hash bytea unique,
                   expires_at, revoked_at, created_by)
idempotency_keys(user_id, key text, request_hash bytea, response_status int, response_body jsonb,
                 created_at, primary key(user_id, key))  -- TTL 24 h, limpiado por job
```

Índices clave: `appointments(business_id, start_at)`, `appointments(professional_id, start_at)`,
`appointments(client_id, start_at desc)`.

### Notifications
```sql
devices(id, user_id, platform, app_kind, push_token text unique, last_seen_at, revoked_at)
consents(id, subject_type, subject_id, channel, phone, granted bool, source, policy_version,
         granted_at, revoked_at)
notification_preferences(subject_type, subject_id, event_type, channel, enabled)
notification_messages(id, business_id, appointment_id null, channel, template, recipient_ref,
                      status, provider_message_id unique null, attempts, last_error,
                      idempotency_key text unique, created_at, updated_at)
appointment_action_tokens(token_hash bytea pk, appointment_id, appointment_version, action,
                          phone, expires_at, used_at)
webhook_events(provider text, event_id text, received_at, processed_at, payload_hash bytea,
               primary key(provider, event_id))
```

## 3. Garantía contra doble reserva

`appointments_no_overlap` es la **última línea de defensa**: aunque el código tenga un error,
PostgreSQL rechaza (SQLSTATE `23P01`) dos citas activas del mismo profesional cuyos rangos
ocupados (`[inicio, fin + preparación)`) se crucen. Detalle en `availability-engine.md` §6 y
ADR-0004.

## 4. Migraciones

- Goose, SQL puro, embebidas (`embed.FS`), numeración secuencial (`00001_...sql`).
- Solo hacia adelante en QA/PROD; los `-- +goose Down` existen para DEV.
- Cambios incompatibles con patrón **expand → migrate → contract** en releases separados para
  permitir rollback de la aplicación sin rollback del esquema.
- La migración corre como paso explícito del despliegue (`api migrate up`), no al arrancar.

## 5. Aislamiento multiempresa a nivel de datos

- MVP: filtro obligatorio `business_id` en cada consulta SQLC (revisado en code review y probado
  con suites cross-tenant).
- Row-Level Security de PostgreSQL queda **diferido** (ADR-0007) — se reevaluará en Fase 12.

## 6. Usuarios de base de datos

| Rol | Uso | Permisos |
|---|---|---|
| `app_migrator` | Migraciones | DDL sobre el esquema |
| `app_runtime` | API y worker | DML (SELECT/INSERT/UPDATE/DELETE), sin DDL |

Implementación QA (T3.2): `deploy/compose/qa/initdb/10-app-roles.sh` (privilegios por defecto de `app_migrator` hacia `app_runtime`; extensiones creadas por el superusuario). Ver `docs/operations/qa-deployment.md` §2.
| `app_readonly` | Soporte/diagnóstico | SELECT |
