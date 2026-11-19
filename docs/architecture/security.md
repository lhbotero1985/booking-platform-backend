# Seguridad: autenticación, autorización y protección

Estado: **Propuesta — Fase 0.** ADR-0006 y ADR-0007.

## 1. Autenticación

### Contraseñas
- **Argon2id** (`golang.org/x/crypto/argon2`), formato PHC
  (`$argon2id$v=19$m=65536,t=3,p=2$salt$hash`), sal aleatoria de 16 bytes, hash de 32 bytes.
  Parámetros por encima del mínimo OWASP (m=19 MiB, t=2, p=1); se calibrarán para ~100–250 ms en
  el Droplet de QA. Rehash transparente al iniciar sesión si cambian los parámetros.
- Política: mínimo 10 caracteres, máximo 128, rechazo de contraseñas comunes, sin reglas de
  composición forzada.
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
| `PLATFORM_ADMIN` | Global (geografía, negocios, usuarios, auditoría). Sin acceso a datos de citas salvo auditoría. |
| `OWNER` | Su negocio: todo, incluida configuración y membresías. |
| `MANAGER` | Su negocio: operación (agenda, servicios, profesionales, clientes); no membresías ni suspensión. |
| `PROFESSIONAL` | Su propia agenda dentro del negocio. |
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

### Enlaces compartidos de agenda
Token opaco hasheado, alcance mínimo (un día de agenda o una cita), solo lectura, expiración
obligatoria (≤ 7 días), revocable, sin datos de contacto del cliente (solo nombre de pila y
servicio).

## 3. Protecciones transversales

| Control | Implementación |
|---|---|
| Validación de entrada | DTO con validación explícita; límites de tamaño de body (1 MiB); UUIDs parseados; enums cerrados; rechazo de campos desconocidos (`DisallowUnknownFields`). |
| SQL injection | Solo SQLC/pgx parametrizado; prohibido concatenar SQL (regla de lint + revisión). |
| Rate limiting | Token bucket en memoria por IP y por usuario (MVP, una instancia). Límites estrictos en login, registro, recuperación y webhooks. Se moverá a PostgreSQL/Redis si se escala horizontalmente. |
| Fuerza bruta | Bloqueo progresivo por cuenta+IP tras intentos fallidos (backoff exponencial, no bloqueo permanente). |
| CORS | Lista blanca de orígenes por ambiente; nunca `*` con credenciales. |
| Cabeceras | HSTS (en Caddy), `X-Content-Type-Options`, `Referrer-Policy`, `Cache-Control: no-store` en respuestas autenticadas. |
| Webhooks | Verificación HMAC-SHA256 (`X-Hub-Signature-256`) sobre el cuerpo crudo con el App Secret de Meta, comparación en tiempo constante, rechazo si falla; deduplicación por ID de evento. |
| Secretos | Variables de entorno inyectadas en el servidor; nunca en Git; `.env.example` sin valores; gitleaks en pre-commit y CI. |
| Logs | Sin contraseñas, tokens, cuerpos de webhooks ni teléfonos completos (enmascarados `+57******4567`). |
| Auditoría | Login/logout, fallos de login, cambios de contraseña, cambios de membresía/rol, cambios de configuración del negocio, suspensiones, acciones admin. |
| Errores | RFC 9457 sin stack traces ni mensajes internos hacia el cliente. |
| Dependencias | govulncheck, Dependabot, Trivy (ver `devsecops.md`). |

## 4. Datos personales

- Minimización: solo nombre, email, teléfono y datos de citas.
- El consentimiento de WhatsApp se registra con versión de política y fecha.
- Derecho de supresión: anonimización del usuario conservando citas históricas del negocio
  (❓ confirmar marco legal por país del piloto, p. ej. Ley 1581 de 2012 en Colombia, LFPDPPP en
  México, GDPR si aplica).
- Ambientes no productivos usan **solo datos sintéticos**.
