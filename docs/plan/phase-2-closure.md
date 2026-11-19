# Cierre de la Fase 2 — Autenticación y autorización

Fecha: 2026-11-19 · Estado: **🟡 Pendiente de aprobación del usuario**

## 1. Entregables

| Tarea | Entregable | Estado | Evidencia |
|---|---|---|---|
| T2.1 | Módulo `identity` (domain / application / infrastructure / interfaces / contract), migración 00003, SQLC, reglas depguard del módulo | ✅ | PR #11 |
| T2.2 | Dominio (`Email`, política de contraseña, `User`), Argon2id (PHC, rehash, límites anti-DoS) | ✅ | PR #12 |
| T2.3 | Registro sin enumeración + verificación obligatoria de email (token de un solo uso, River, SMTP/Mailpit) | ✅ | PR #13 |
| T2.4 | Login, sesiones opacas (Bearer móvil / cookie `__Host-session` web), CSRF por Origin, logout, logout-all, `/me` | ✅ | PR #14 |
| T2.5 | Recuperación y cambio de contraseña, revocación de sesiones, aviso por email | ✅ | PR #17 |
| T2.6 | RBAC: `contract.Authorizer`, políticas puras, 403 mismo negocio / 404 otro negocio, `/me/memberships` | ✅ | PR #18 |
| T2.7 | Rate limiting (general y estricto) y backoff de login, 429 `RATE_LIMITED` + `Retry-After` | ✅ | PR #19 |
| T2.8 | Auditoría (`audit_log`) transaccional, sin datos sensibles | ✅ | PR #20 |
| T2.9 | Suite de seguridad de la fase, revisión ASVS, este documento | ✅ | `feature/t2.9-security-closure` |

API: 13 operaciones documentadas en `api/openapi.yaml` y validadas por pruebas de contrato.

## 2. Pruebas

| Tipo | Estado | Evidencia (`docs/testing/test-results.md`) |
|---|---|---|
| Unitarias | ✅ | Cobertura: domain 100 %, worker 100 %, ratelimit 98.8 %, argon2 95.8 %, httpx 93.7 %, interfaces/http 85.2 %, application 84.6 %, contract 80 % |
| Integración (PostgreSQL real, testcontainers) | ✅ | Registro, sesiones, contraseñas, membresías, auditoría (incluido rollback atómico) |
| Contrato OpenAPI | ✅ | Todas las operaciones de identity, incluidos 401/403/415/429 |
| E2E Docker Compose + Mailpit | ✅ | Un escenario por tarea (T2.3–T2.8) |
| Concurrencia | ✅ | Limitador con 200 goroutines; `-race` en CI |
| Seguridad (`tests/security`) | ✅ | 17 pruebas sobre el stack HTTP completo (ver §3) |
| Análisis estático y dependencias | ✅ | golangci-lint (gosec, depguard), govulncheck, gitleaks, Trivy |

## 3. Checklist de seguridad de la Fase 2

| Control | Verificación |
|---|---|
| Rutas autenticadas: sin credencial, Basic, basura, token de verificación/reset, token forjado o truncado, sesión revocada, expirada por inactividad, usuario suspendido, cookie inválida → 401 + `WWW-Authenticate` + `no-store` | `TestAuthenticatedRoutesRejectInvalidCredentials` (5 rutas × 12 casos) |
| Token nuevo y aleatorio en cada login; sin fijación de sesión | `TestSessionTokensAreFreshAndOpaque` |
| Cookie `__Host-session`: HttpOnly, Secure, SameSite=Lax, Path=/, sin Domain; token fuera del cuerpo | `TestWebSessionCookieIsHardened` |
| CSRF: escrituras con cookie exigen Origin exacto (faltante, ajeno, parecido, http → 403) | `TestCookieSessionsRequireAllowedOrigin` |
| Asignación masiva (`isPlatformAdmin`, `status`, `emailVerifiedAt`, `id`) rechazada | `TestMassAssignmentIsRejected` |
| Anti-enumeración: registro, reenvío, olvido y login idénticos para cuenta activa, pendiente e inexistente | `TestPublicEndpointsDoNotRevealAccounts` |
| Tiempo de login comparable con email inexistente (Argon2id real) | `TestLoginTimingDoesNotRevealAccounts` |
| Tokens ligados a su propósito y no consumidos por un uso indebido | `TestTokensAreBoundToTheirPurpose` |
| Contraseñas, tokens y emails nunca en logs ni en `audit_log` | `TestSecretsNeverReachLogsOrAudit` |
| Fuerza bruta con la configuración por defecto: ≤ 5 verificaciones por minuto desde una IP | `TestBruteForceIsStoppedWithDefaultLimits` |
| Matriz rol × acción; otro negocio → 404 idéntico a ruta inexistente; revocación inmediata | `contract` y `interfaces/http` (T2.6), integración |
| Tokens solo como hash SHA-256 en BD; nunca en la cola de jobs | Integración T2.3/T2.5, `worker_test.go` |
| Auditoría atómica con el cambio | `TestAuditIsAtomicWithTheChange`, `TestAuditRollsBackWithTheChange` |
| Controles heredados de la Fase 1 (cabeceras, límites, CORS, errores sin internos) | `tests/security/baseline_test.go` |

## 4. Revisión OWASP ASVS 4.0.3 — nivel 2 (V2, V3, V4)

Leyenda: ✅ cumple · ⚠️ brecha (requiere decisión) · ➖ no aplica aún.

| Req. | Requisito (resumen) | Estado | Detalle |
|---|---|---|---|
| V2.1.1 | Contraseñas de al menos 12 caracteres | ⚠️ | La política aprobada es 10 (security.md §1). **P31** |
| V2.1.2 | Permitir 64+ caracteres, máximo 128 | ✅ | 10–128 |
| V2.1.3 | Sin truncado | ✅ | Más de 128 se rechaza, nunca se trunca |
| V2.1.4 | Cualquier carácter Unicode imprimible, incluidos espacios | ✅ | Frases con espacios en las pruebas |
| V2.1.7 | Verificar contra contraseñas filtradas (top 1.000–10.000 o servicio) | ⚠️ | Lista local pequeña. **P31** |
| V2.1.9 | Sin reglas de composición | ✅ | |
| V2.2.1 | Controles anti-automatización | ✅ | T2.7; ataques distribuidos en **P28** |
| V2.2.3 | Notificación tras cambios de credenciales | ✅ | Email "Tu contraseña fue cambiada" |
| V2.4.1 | Almacenamiento resistente a ataques offline | ✅ | Argon2id 64 MiB / t=3 / p=2 (calibración en QA, P27) |
| V2.5.2 | Sin pistas ni preguntas de seguridad | ✅ | |
| V2.5.4 | Sin cuentas compartidas o por defecto | ✅ | |
| V2.5.6 | Recuperación con token temporal de un solo uso | ✅ | 30 min, revoca sesiones |
| V2.8 / V4.3.1 | MFA (recomendado para administración) | ⚠️ | Fuera del MVP; propuesta para PLATFORM_ADMIN. **P32** |
| V3.1.1 | Token de sesión nunca en la URL | ✅ | Header o cookie |
| V3.2.1 | Token nuevo al autenticar | ✅ | `TestSessionTokensAreFreshAndOpaque` |
| V3.2.2 | Al menos 64 bits de entropía | ✅ | 256 bits |
| V3.2.3 | Almacenamiento seguro en el navegador | ✅ | Cookie HttpOnly; nada en `localStorage` |
| V3.3.1 | Logout invalida la sesión en el servidor | ✅ | |
| V3.3.2 | Expiración por inactividad y absoluta | ✅ | Web 12 h / 7 d · móvil 30 d / 90 d · admin 1 h / 12 h |
| V3.3.3 | Cerrar otras sesiones tras cambiar la contraseña | ✅ | Automático |
| V3.3.4 | El usuario puede ver y cerrar sus sesiones activas | ⚠️ | Solo logout-all; listado no expuesto. **P33** |
| V3.4.1–V3.4.5 | Cookie Secure, HttpOnly, SameSite, prefijo `__Host-`, Path | ✅ | |
| V3.5.x | Tokens de sesión | ✅ | Opacos y revocables (sin JWT, ADR-0006) |
| V3.7.1 | Reautenticación antes de cambios sensibles | ✅ | Cambio de contraseña exige la actual |
| V4.1.1 | Control de acceso en el servidor | ✅ | Casos de uso + `contract.Authorizer` |
| V4.1.2 | Atributos de control no manipulables | ✅ | `TestMassAssignmentIsRejected` |
| V4.1.3 | Mínimo privilegio | ✅ | Administrador sin rol implícito en negocios |
| V4.1.5 | Fallo seguro | ✅ | Lista vacía o rol desconocido → denegado |
| V4.2.1 | Protección IDOR | ✅ (base) | Patrón 404; cada módulo de negocio lo aplica y prueba desde la Fase 4 |
| V4.2.2 | Anti-CSRF | ✅ | SameSite=Lax + Origin |
| V7.1.1–V7.1.2 | Sin credenciales ni datos sensibles en logs; eventos de seguridad registrados | ✅ | Redacción slog + `audit_log` |

## 5. Riesgos y pendientes (no bloqueantes salvo decisión)

| # | Pendiente | Plan |
|---|---|---|
| 1 | P31 — Política de contraseñas: subir mínimo a 12 y ampliar la lista a ~10.000 filtradas | Decisión del usuario (§6) |
| 2 | P32 — MFA para PLATFORM_ADMIN | Propuesta: TOTP antes de PROD (Fase 12) |
| 3 | P33 — Listado y cierre selectivo de sesiones | Endpoint pequeño (`GET/DELETE /me/sessions`); la consulta ya existe |
| 4 | P28 — Ataques distribuidos | Fase 12 |
| 5 | P29 — NAT; IP real tras Caddy | Fase 3 |
| 6 | P27 — Calibración Argon2id | Fase 3 (Droplet QA) |
| 7 | P30 — Retención de `audit_log` | Fase 12 |
| 8 | UI de Mailpit en QA solo por túnel SSH | Fase 3 |
| 9 | `main` sigue en T1.3; Dependabot lee su configuración desde `main` | PR `qa` → `main` tras validar en QA (Fase 3) |
| 10 | Rate limiting en memoria (una instancia) | Mover a almacenamiento compartido si se escala horizontalmente |

## 6. Decisión requerida

1. ¿Cómo resolver P31 (V2.1.1 / V2.1.7)? Opciones: **(a)** subir el mínimo a 12 y ampliar la
   lista de contraseñas comunes a ~10.000 ahora, como tarea corta T2.10, antes de cerrar; **(b)**
   mantener 10 y documentar la excepción; **(c)** posponer a la Fase 12.
2. ¿P32 (MFA admin) y P33 (sesiones) para la Fase 12?
3. Aprobar el cierre de la Fase 2 y autorizar el inicio de la **Fase 3 — Infraestructura QA**
   (Droplet en DigitalOcean: crea recursos con costo y requiere autorización explícita).
