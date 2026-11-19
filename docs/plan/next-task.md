# Siguiente tarea

## Última tarea completada
T2.2 — Dominio de identidad y Argon2id (2026-11-19), en `feature/t2.2-identity-domain-argon2`
(commit y push hechos; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente. PR, merge,
borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.2-identity-domain-argon2` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t2.1-identity-skeleton` (local y remota).

## Próxima tarea: T2.3 — Registro, login, sesiones y logout

- **Objetivo:** primeros endpoints de negocio con tokens opacos (ADR-0006).
- **Archivos:**
  - `internal/platform/clock` (puerto `Clock` + implementación real y fija para pruebas).
  - `identity/domain`: `Session` (expiración por inactividad y absoluta por tipo de cliente),
    generación de token (`bp_s_` + 32 bytes aleatorios base64url) y su hash SHA-256.
  - `identity/application`: casos de uso `Register`, `Login` (con `VerifyDummy` si el email no
    existe, rehash transparente), `Logout`, `LogoutAll`; puertos de repositorio.
  - `identity/infrastructure/postgres`: repositorios sobre `sqlcgen` y `db.WithTx`.
  - `identity/interfaces/http`: `POST /api/v1/auth/register`, `/login`, `/logout`,
    `/logout-all`; DTO, validación, mapeo de errores (409 email registrado, 401 genérico).
  - `api/openapi.yaml`: operaciones y esquemas nuevos.
  - Composition root (`internal/app`) cableando el módulo.
- **Pruebas:** unitarias de casos de uso con fakes; integración con PostgreSQL; contrato
  OpenAPI; seguridad: mismo mensaje y tiempo comparable para email inexistente y contraseña
  incorrecta, el token nunca se guarda ni se registra en logs, usuario suspendido no inicia
  sesión.
- **Pregunta abierta:** confirmar que el registro deja la cuenta `ACTIVE` sin verificación de
  email en el MVP (propuesta del dominio actual).
