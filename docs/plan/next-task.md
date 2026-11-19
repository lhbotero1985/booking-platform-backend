# Siguiente tarea

## Última tarea completada
T2.8 — Auditoría básica (2026-11-19), en `feature/t2.8-audit` (commit y push hechos; PR y
merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.8-audit` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t2.7-rate-limiting` (local y remota).

## Próxima tarea: T2.9 — Suite de seguridad de la fase y cierre de la Fase 2

- **Objetivo:** verificar de extremo a extremo los controles de la Fase 2 y preparar su
  aprobación (mismo formato que `phase-1-closure.md`).
- **Alcance propuesto:**
  - `tests/security` ampliada sobre el stack HTTP completo: matriz de rutas autenticadas (sin
    token, token de otro tipo, sesión revocada/expirada, usuario suspendido); fijación de sesión
    (el token cambia en cada login); cookie `__Host-session` endurecida; CSRF; asignación
    masiva (`isPlatformAdmin`, `status`) rechazada; anti-enumeración (registro, reenvío,
    olvido, login y 429 idénticos); tokens de un tipo no sirven para otro; sin secretos en
    logs ni en `audit_log`; cabeceras `Cache-Control: no-store` en respuestas autenticadas.
  - Revisión de OWASP ASVS nivel 2 (capítulos V2 autenticación, V3 sesiones, V4 control de
    acceso) con estado por requisito.
  - `docs/plan/phase-2-closure.md`: entregables, checklist de seguridad, pruebas, riesgos y
    preguntas abiertas (P27–P30), y lo que se pospone a la Fase 3 (IP real tras Caddy, Mailpit
    por túnel SSH, calibración de Argon2).
- **Criterio:** todo en verde en CI y aprobación explícita del usuario para cerrar la Fase 2.
