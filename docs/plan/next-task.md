# Siguiente tarea

## Última tarea completada
T2.6 — RBAC y autorización por recurso (2026-11-19), en `feature/t2.6-rbac`
(commit y push hechos; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.6-rbac` → `qa` y merge si el CI queda en verde.

## Próxima tarea: T2.7 — Rate limiting y defensa contra fuerza bruta

- **Objetivo:** limitar abuso en endpoints públicos sensibles (security.md §3).
- **Diseño propuesto:**
  - Token bucket en memoria (`golang.org/x/time/rate` o implementación propia mínima) por IP y,
    en login, por cuenta (email normalizado, hasheado en memoria); limpieza periódica de
    entradas inactivas y tope de entradas (anti-DoS de memoria).
  - Límites estrictos en `login`, `register`, `verify-email/resend`, `password/forgot`,
    `password/reset`, `verify-email`; límite general más amplio para el resto.
  - Backoff progresivo por cuenta+IP tras fallos de login (exponencial con tope, sin bloqueo
    permanente); respuesta 429 `RATE_LIMITED` con `Retry-After`, sin revelar si la cuenta existe.
  - Configurable por variables de entorno; IP del cliente = par TCP (X-Forwarded-For solo desde
    Caddy en la Fase 3).
- **Pruebas:** límites por ruta, recuperación del bucket con reloj inyectado, backoff, 429
  idéntico para cuentas existentes e inexistentes, contrato OpenAPI (429), concurrencia.
