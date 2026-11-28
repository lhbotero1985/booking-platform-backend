# Siguiente tarea

## Última tarea completada
Reseñas y calificaciones de negocios y profesionales (2026-11-28, ADR-0021), en `qa` (PR #84) y
validadas en QA: E2E 266/266. El frontend debe crear la página para calificar (recibe
`?appointmentId=`), mostrar `rating` en la página pública, la búsqueda y el catálogo, y la lista de
reseñas; con esa página se configura `APPOINTMENT_REVIEW_URL` en QA (P45, junto con el texto del
correo).

Anterior: Imágenes del negocio (logo, portada) y portafolio de los profesionales (2026-11-27), en `qa`
(PR #82, 2026-11-28) y validadas en QA: E2E 257/257. El frontend puede usar `logoUrl`, `coverUrl`,
el portafolio y las subidas `multipart/form-data` (campo `image`). Antes de PROD: P44 (respaldo
del volumen `media` o Spaces).

Anterior: P-11 y P-12 — Más negocios por dueño y estadísticas (2026-11-27), en `qa` (PR #80) y validadas
en QA: E2E 248/248. El frontend debe enviar `Idempotency-Key` en «Crear otro negocio», explicar
`OWNED_BUSINESS_LIMIT` y registrar P-11 y P-12 como implementadas en su documento.

Anterior: Operación QA (2026-11-27): botón «Reagendar» de los correos hacia
`https://booka-app-qa.duckdns.org/citas/reagendar`, en `qa` (PR #78) y validado en QA (E2E 239
PASS, 0 fallos, con `RESCHEDULE_URL`).

Anterior: P-9 y P-10 — Horas libres del personal y citas de un cliente (2026-11-26), en `qa` (PR #76) y
validadas en QA: E2E 241/241 (con `APP_URL=https://booka-app-qa.duckdns.org`).

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
