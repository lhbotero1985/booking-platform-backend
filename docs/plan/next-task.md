# Siguiente tarea

## Última tarea completada
T2.9 — Suite de seguridad y cierre de la Fase 2 (2026-11-19), en
`feature/t2.9-security-closure` (commit y push hechos; PR y merge pendientes de autorización).

**La Fase 2 está pendiente de aprobación** (`docs/plan/phase-2-closure.md`). No se inicia la
Fase 3 sin aprobación explícita.

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización / decisión

1. PR `feature/t2.9-security-closure` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t2.8-audit` (local y remota).
3. P31 — política de contraseñas frente a ASVS L2: (a) T2.10 ahora con mínimo 12 y lista de
   ~10.000 contraseñas comunes; (b) mantener 10 y documentar la excepción; (c) Fase 12.
4. P32 (MFA para administradores) y P33 (listado de sesiones): ¿Fase 12?
5. Aprobar el cierre de la Fase 2.

## Próxima fase (tras aprobación): Fase 3 — Infraestructura QA

Se desglosará en tareas al iniciarla. Incluye recursos con costo en DigitalOcean (Droplet), que
requieren autorización explícita: Droplet QA, Caddy (TLS, IP real), roles de BD
`app_migrator`/`app_runtime`, publicación de imagen en GHCR con SBOM, despliegue desde `qa`,
Mailpit por túnel SSH, calibración de Argon2id (P27), backups.
