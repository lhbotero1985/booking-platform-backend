# Siguiente tarea

## Última tarea completada
P-6 — Correos de identidad con la marca Booka (2026-11-26), en `feature/p6-identity-emails`
(commit y push; PR pendiente de autorización). Fase 7 aprobada por el usuario (2026-11-25).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

El usuario se dedica al frontend (otro agente). En el backend no hay tarea autorizada:
1. **No iniciar otra fase sin aprobación explícita.** Candidata de backend: Fase 11B
   (suscripciones y pagos, aprobada en alcance el 2026-11-24, sin iniciar).
2. Atender lo que el frontend necesite del contrato de la API (`docs/plan/frontend-contract-proposals.md`).
   P-6: PR a `qa` y revisión en Mailpit (requiere autorización); cambiar `PUBLIC_APP_URL` de QA a
   `https://booka-app-qa.duckdns.org` cuando el frontend publique `/verify-email` y
   `/reset-password` (autorización).
3. Cuando Meta apruebe las plantillas: prueba real de mensajes y botones de WhatsApp; plantilla
   nueva con "especialista" (P42).
