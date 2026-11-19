# Siguiente tarea

## Última tarea completada
T2.3 — Registro y verificación de email (2026-11-19), en `feature/t2.3-register-verify-login`
(commit y push hechos; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.3-register-verify-login` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t2.2-identity-domain-argon2` (local y remota).

## Próxima tarea: T2.4 — Login, sesiones y middleware de autenticación

- **Objetivo:** iniciar sesión solo con cuentas verificadas y proteger endpoints (ADR-0006).
- **Archivos:**
  - `identity/domain`: `Session` (expiración por inactividad y absoluta según `MOBILE`/`WEB`,
    renovación deslizante acotada, revocación).
  - `identity/application`: `Login` (email inexistente → `VerifyDummy`; contraseña incorrecta y
    email inexistente → mismo error y tiempo; cuenta pendiente → `EMAIL_NOT_VERIFIED` solo tras
    contraseña correcta; suspendida → 401 genérico; rehash transparente), `Authenticate(token)`,
    `Logout`, `LogoutAll`, `Me`; `last_seen` con throttling (≥ 5 min).
  - `identity/contract`: `Principal` y extracción desde el contexto para otros módulos.
  - `identity/interfaces/http`: `POST /api/v1/auth/login`, `/logout`, `/logout-all`,
    `GET /api/v1/me`; middleware de autenticación (Bearer para móvil; cookie `__Host-session`
    `HttpOnly; Secure; SameSite=Lax` + verificación de `Origin`/CSRF para web).
  - OpenAPI: operaciones nuevas y `security` aplicado a endpoints protegidos.
- **Pruebas:** matriz 401 (sin token, malformado, inexistente, expirado por inactividad,
  expirado absoluto, revocado, usuario suspendido); tiempo comparable de login con email
  inexistente vs contraseña incorrecta; token de sesión nunca persistido ni registrado; logout-all
  revoca todas; contrato; E2E en compose (registro → verificación → login → /me → logout).
