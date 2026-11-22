# Siguiente tarea

## Última tarea completada
T2.10 — Política de contraseñas, mínimo 8 (2026-11-19), en `feature/t2.10-password-policy`
(commit y push hechos; PR y merge pendientes de autorización).

**Fase 2 aprobada por el usuario (2026-11-19).**

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.10-password-policy` → `qa` y merge si el CI queda en verde.
2. Inicio de la **Fase 3 — Infraestructura QA**.

## Próxima fase: Fase 3 — Infraestructura QA (requiere autorización)

Al iniciarla se desglosará en tareas y se presentará el costo estimado. Incluye recursos con
costo en DigitalOcean (Droplet QA), por lo que cada creación de recursos se pedirá por separado:
Caddy (TLS, IP real del cliente), roles de BD `app_migrator`/`app_runtime`, imagen en GHCR con
SBOM, despliegue desde `qa`, Mailpit solo por túnel SSH, calibración de Argon2id (P27), backups,
y PR `qa` → `main` tras validar en QA.
