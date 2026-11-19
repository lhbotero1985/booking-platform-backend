# Siguiente tarea

## Última tarea completada
T1.9 — Seguridad básica y cierre de la Fase 1 (2026-11-19), en `feature/t1.9-phase1-closure`.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente. PR, merge,
borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR de `feature/t1.9-phase1-closure` → `qa` (el CI valida también la integración, que no se
   pudo repetir localmente porque Docker Desktop se detuvo) y merge si queda en verde.
2. **Aprobación del cierre de la Fase 1** (`docs/plan/phase-1-closure.md`).

## Próxima tarea (tras aprobar la Fase 1): T2.1 — Esqueleto del módulo `identity`

- **Objetivo:** primer módulo de negocio con las capas de Clean Architecture y su esquema.
- **Archivos:** `internal/modules/identity/{domain,application,infrastructure,interfaces/http,contract}`,
  `db/migrations/00003_identity.sql` (`users`, `sessions`, `password_reset_tokens`,
  `memberships`, `audit_log`), `db/queries/identity/*.sql`, entrada en `sqlc.yaml`,
  reglas depguard del módulo (dominio puro, sin importar otros módulos salvo `contract`).
- **Pruebas:** migración up/down/up; `sqlc diff`; violación intencional de depguard detectada.
- **Pregunta abierta para la Fase 2:** P13 proveedor de email (recuperación de contraseña).
