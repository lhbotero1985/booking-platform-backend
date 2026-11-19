# Siguiente tarea

## Última tarea completada
T2.1 — Esqueleto del módulo identity (2026-11-19), en `feature/t2.1-identity-skeleton`
(commit y push hechos; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente. PR, merge,
borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.1-identity-skeleton` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t1.9-phase1-closure` (local y remota).

## Próxima tarea: T2.2 — Dominio de identidad y Argon2id

- **Objetivo:** modelo de dominio puro y hashing de contraseñas seguro.
- **Archivos:** `internal/modules/identity/domain/` (`Email`, `Password` con política,
  `UserStatus`, `Role`, `User` con invariantes, errores de dominio), puerto `PasswordHasher` en
  `application`, implementación Argon2id en `infrastructure/argon2` (formato PHC, sal de 16 bytes,
  comparación en tiempo constante, `NeedsRehash`), parámetros en config (`ARGON2_*`).
- **Pruebas:** tablas de casos para VO (emails válidos/inválidos, normalización, contraseñas
  cortas/largas/comunes); hash/verify; rechazo de hashes malformados; rehash al cambiar
  parámetros; benchmark para calibrar ≈100–250 ms; fuzzing del parser PHC.
- **Dependencia nueva:** `golang.org/x/crypto/argon2`.
- **Criterios de aceptación:** dominio sin dependencias externas (depguard); cobertura del
  dominio ≥ 90 %; tiempos de hash medidos y registrados.
