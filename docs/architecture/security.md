# Seguridad: autenticación, autorización y protección

Estado: **Propuesta — Fase 0.** ADR-0006 y ADR-0007.

## 1. Autenticación

### Contraseñas
- **Argon2id** (`golang.org/x/crypto/argon2`), formato PHC
  (`$argon2id$v=19$m=65536,t=3,p=2$salt$hash`), sal aleatoria de 16 bytes, hash de 32 bytes.
  Parámetros por encima del mínimo OWASP (m=19 MiB, t=2, p=1); se calibrarán para ~100–250 ms en
  el Droplet de QA. Rehash transparente al iniciar sesión si cambian los parámetros.
- Política: mínimo 8 caracteres (mínimo NIST SP 800-63B), máximo 128, rechazo de una lista
  pequeña de contraseñas comunes, sin reglas de composición forzada. **Excepción documentada
  frente a OWASP ASVS L2** (V2.1.1 pide 12; V2.1.7 una lista amplia de filtradas): decisión del
  usuario 2026-11-19 (P31), compensada con Argon2id, rate limiting y backoff. Las contraseñas
  anteriores siguen siendo válidas para iniciar sesión.
- Comparaciones en tiempo constante. Mensajes de error genéricos ("credenciales inválidas").
- Mitigación de enumeración: el login con email inexistente ejecuta un hash ficticio.

### Tokens opacos y sesiones
- Token = 32 bytes de `crypto/rand`, codificado base64url, con prefijo identificable
  (`bp_s_…`) para que los escáneres de secretos lo detecten si se filtra.
- En BD solo se guarda `SHA-256(token)`; un volcado de BD no permite suplantar sesiones.
- Expiración:

| Cliente | Inactividad | Absoluta |
|---|---|---|
| Móvil (cliente / profesional) | 30 días | 90 días |
| Web (negocio / admin) | 12 horas | 7 días |
| Admin plataforma | 1 hora | 12 horas |

- `last_seen_at` se actualiza como máximo cada 5 min (evita una escritura por request).
- Revocación: logout (sesión actual), "cerrar todas las sesiones", cambio/restablecimiento de
  contraseña (revoca todas), suspensión de usuario (revoca todas).
- Transporte: móvil → `Authorization: Bearer`; web → cookie `__Host-session` `HttpOnly; Secure;
  SameSite=Lax` + protección CSRF (token double-submit o verificación de `Origin`) en métodos
  mutantes. (❓ confirmar si los portales Next.js llamarán a la API directamente o vía BFF.)

### Recuperación de contraseña
- Token aleatorio de un solo uso, hasheado, válido 30 min, enviado por email (proveedor a definir
  ❓) — la respuesta del endpoint es siempre 202, exista o no el email.

## 2. Autorización

### Roles
| Rol | Alcance |
|---|---|
| `PLATFORM_ADMIN` | Global: geografía, usuarios, auditoría y **cualquier acción sobre cualquier negocio** (ADR-0018, auditada con el administrador como actor). |
| `OWNER` | Responsable de su negocio (puede haber varios, siempre al menos uno): todo — configuración, miembros, catálogo, profesionales, horarios. Puede también atender como barbero. |
| `PROFESSIONAL` | Barbero o estilista: agenda citas a clientes (Fase 6), ajusta sus propios descansos y registra sus propias ausencias. |
| `CUSTOMER` (implícito) | Sus propias citas, perfil y dispositivos. |

### Modelo de verificación
1. **Middleware de autenticación**: resuelve la sesión → `Principal{UserID, IsPlatformAdmin}`.
2. **Caso de uso**: cada comando/query recibe el `Principal` y llama a una **política explícita**
   (`CanCancelAppointment(principal, appointment, memberships)`). Las políticas son funciones
   puras y probadas unitariamente.
3. **Autorización por recurso**: el recurso se carga **acotado al negocio** (`WHERE id=$1 AND
   business_id=$2`). Un recurso de otro negocio responde **404** (no 403) para no revelar
   existencia (protección IDOR).
4. Las membresías se cargan por request (consulta indexada) — sin cachear permisos en el token
   para que la revocación sea inmediata.

### Implementación (T2.6)

- `identity/contract`: `Role`, `Membership`, puerto `Authorizer` y las políticas puras
  `CheckBusinessRole` / `CheckPlatformAdmin`. Otros módulos reciben un `contract.Authorizer`
  (implementado por el servicio de identity) y lo invocan **dentro del caso de uso**, no como
  middleware de ruta.
- Reglas de `RequireBusinessRole(principal, businessID, roles...)`:
  sin membresía activa en ese negocio → `ErrNotFound` (404) · membresía con rol no permitido →
  `ErrForbidden` (403) · lista de roles vacía → `ErrForbidden` (falla cerrado).
- `PLATFORM_ADMIN` puede ejecutar cualquier caso de uso de cualquier negocio (decisión del
  usuario, ADR-0018; reemplaza "sin rol implícito"). El autorizador lo permite y cada acción queda
  auditada con el administrador como actor. El rol solo se otorga en el servidor (`api admin`).
- Errores compartidos en `internal/platform/authz`; `httpx.WriteAuthzProblem` los traduce. El
  404 es idéntico al de una ruta inexistente (sin `detail`).
- `GET /api/v1/me/memberships` lista los negocios y roles activos del usuario.

### Matriz de políticas (conjuntos de roles)

| Conjunto | Roles | Acciones (Fase 4 en adelante) |
|---|---|---|
| `RolesOwner` | OWNER (responsable; puede haber varios, siempre al menos uno) | Configuración del negocio, miembros y roles, catálogo, profesionales y horarios (el rol MANAGER se eliminó en T5.1) |
| `RolesStaff` | OWNER, PROFESSIONAL | Ver el negocio, agenda, descansos y ausencias propios; el caso de uso además compara el dueño del recurso con el principal |
| — (sin membresía) | CUSTOMER | Solo recursos propios (`/me/...`); nunca datos internos de un negocio |
| `RequirePlatformAdmin` | PLATFORM_ADMIN | Geografía, alta/suspensión de negocios, auditoría. El rol solo se otorga con `api admin grant-platform-admin` en el servidor (auditado); se lee en cada petición, así que revocarlo es inmediato |

| Caso | Mismo negocio, rol permitido | Mismo negocio, rol insuficiente | Otro negocio / sin membresía |
|---|---|---|---|
| Respuesta | 200 | 403 `FORBIDDEN` | 404 `NOT_FOUND` |

### Enlaces compartidos de agenda
Token opaco hasheado, alcance mínimo (un día de agenda o una cita), solo lectura, expiración
obligatoria (≤ 7 días), revocable, sin datos de contacto del cliente (solo nombre de pila y
servicio).

## 3. Protecciones transversales

| Control | Implementación |
|---|---|
| Validación de entrada | DTO con validación explícita; límites de tamaño de body (1 MiB); UUIDs parseados; enums cerrados; rechazo de campos desconocidos (`DisallowUnknownFields`). |
| SQL injection | Solo SQLC/pgx parametrizado; prohibido concatenar SQL (regla de lint + revisión). |
| Rate limiting | Token bucket en memoria por cliente (IPv4 o prefijo IPv6 /64), acotado en claves (`RATE_LIMIT_MAX_KEYS`). General: 300/min, ráfaga 100 (sondas `/healthz` y `/readyz` exentas). Estricto y compartido por login, registro, verificación, reenvío, olvido/restablecimiento y cambio de contraseña: 10/min, ráfaga 10. Respuesta 429 `RATE_LIMITED` + `Retry-After`. Webhooks: se agregarán en su fase. Se moverá a PostgreSQL/Redis si se escala horizontalmente. (T2.7) |
| Fuerza bruta | Backoff por cuenta+IP (hash del email enviado, exista o no la cuenta): desde el 5.º fallo consecutivo, bloqueo de 1 s duplicándose hasta 15 min; nunca permanente; un acierto lo reinicia. Se evalúa **antes** de Argon2 (un atacante bloqueado no consume CPU). El cambio de contraseña autenticado aplica el mismo backoff por usuario. Ataques distribuidos (muchas IP contra una cuenta) quedan para la Fase 12 (P28). (T2.7) |
| CORS | Lista blanca de orígenes por ambiente; nunca `*` con credenciales. |
| Proxy e IP real | Caddy único punto de entrada (TLS 1.2+, HTTP/3); la API cree `X-Forwarded-For` solo desde `TRUSTED_PROXIES` (IP fija de Caddy), recorriendo la cadena de derecha a izquierda; cadenas malformadas se ignoran; `0.0.0.0/0` rechazado en la configuración (T3.3). |
| Cabeceras | HSTS (en Caddy), `X-Content-Type-Options`, `Referrer-Policy`, `Cache-Control: no-store` en respuestas autenticadas. |
| Webhooks | Verificación HMAC-SHA256 (`X-Hub-Signature-256`) sobre el cuerpo crudo con el App Secret de Meta, comparación en tiempo constante, rechazo si falla; deduplicación por ID de evento. |
| Secretos | Variables de entorno inyectadas en el servidor; nunca en Git; `.env.example` sin valores; gitleaks en pre-commit y CI. |
| Logs | Sin contraseñas, tokens, cuerpos de webhooks ni teléfonos completos (enmascarados `+57******4567`). |
| Auditoría | Tabla `audit_log` (T2.8). Eventos: `auth.registered`, `auth.email_verified`, `auth.login` (SUCCESS / FAILURE / DENIED por backoff o email sin verificar), `auth.logout`, `auth.logout_all`, `auth.password_reset_requested`, `auth.password_reset`, `auth.password_changed`, `authz.denied` (403 y 404 entre negocios). Cambios de membresía/rol, configuración del negocio, suspensiones y acciones admin se agregan en sus fases. Los eventos que acompañan un cambio de datos se escriben **en la misma transacción** (sin cambio sin rastro); los demás, en transacción propia y *best-effort* (un fallo de auditoría se registra en logs y no bloquea la autenticación). Cada fila lleva `request_id`, IP, actor y `reason`; nunca contraseñas, tokens, emails (para cuentas inexistentes, huella SHA-256 truncada `email_fp`), nombres ni user agents. `occurred_at` usa el reloj de la aplicación. Retención y consulta: Fase 12 (P30). |
| Errores | RFC 9457 sin stack traces ni mensajes internos hacia el cliente. |
| Dependencias | govulncheck, Dependabot, Trivy (ver `devsecops.md`). |

## 4. Datos personales

- Minimización: solo nombre, email, teléfono y datos de citas.
- El consentimiento de WhatsApp se registra con versión de política y fecha.
- Derecho de supresión: anonimización del usuario conservando citas históricas del negocio
  (❓ confirmar marco legal por país del piloto, p. ej. Ley 1581 de 2012 en Colombia, LFPDPPP en
  México, GDPR si aplica).
- Ambientes no productivos usan **solo datos sintéticos**.
