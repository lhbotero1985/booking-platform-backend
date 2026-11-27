# Siguiente tarea

## Última tarea completada
P-9 y P-10 — Horas libres del personal y citas de un cliente (2026-11-26), en
`feature/p9-p10-staff-availability-history` (commit y push; PR pendiente de autorización). E2E en
QA con `APP_URL=https://booka-app-qa.duckdns.org`.

Anterior: P-6 — Correos de identidad con la marca Booka (2026-11-26), en `qa` (PR #73) y validada en
Mailpit de QA. Fase 7 aprobada por el usuario (2026-11-25).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea

El usuario se dedica al frontend (otro agente). En el backend no hay tarea autorizada:
1. **No iniciar otra fase sin aprobación explícita.** Candidata de backend: Fase 11B
   (suscripciones y pagos, aprobada en alcance el 2026-11-24, sin iniciar).
2. Atender lo que el frontend necesite del contrato de la API (fuente: `booking-platform-frontend/docs/BACKEND_CONTRACT_PROPOSALS.md`, solo lectura;
   seguimiento en `docs/plan/frontend-contract-proposals.md`).
3. Cuando Meta apruebe las plantillas: prueba real de mensajes y botones de WhatsApp; plantilla
   nueva con "especialista" (P42).
