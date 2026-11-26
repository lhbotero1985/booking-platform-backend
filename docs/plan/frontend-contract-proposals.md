# Propuestas del frontend — seguimiento en el backend

**La fuente de las propuestas es el repositorio del frontend:**
`booking-platform-frontend/docs/BACKEND_CONTRACT_PROPOSALS.md`. Este archivo **no copia** su
contenido (la copia se desfasaba); solo registra qué hizo el backend con cada una (decisión del
usuario, 2026-11-26).

Reglas:
- El backend **solo lee** el documento del frontend; no lo modifica sin autorización (regla 11).
  Las dudas se responden al usuario y el frontend actualiza su documento.
- Leer una propuesta no autoriza implementarla: el usuario indica cuál se inicia y el backend
  presenta el plan antes (formato por tarea).
- Si una propuesta choca con el modelo o rompe el contrato v1, se avisa antes de implementar.

| Propuesta | Brecha | Estado en el backend | Rama / PR | Notas |
|---|---|---|---|---|
| P-6 | G-23 correos de identidad sin marca; G-24 enlace al dominio de la API en QA | ✅ En `qa`, validada en Mailpit (2026-11-26) | PR #73 | Opción A: diseño base en `internal/platform/mail/brand`. `PUBLIC_APP_URL` de QA → `https://booka-app-qa.duckdns.org` aplicado (2026-11-26); PROD: dominio de business-web |
| P-1 | G-13 membresías sin datos del negocio | Sin iniciar | — | — |
| P-2 | G-10 admin no ve geografía inactiva | Sin iniciar | — | — |
| P-3 | G-05 sin `PATCH /me` | Sin iniciar | — | Duda abierta: ¿el nombre del cliente del negocio sigue al de la cuenta? |
| P-4 | G-15 sin detalle de cita para el personal | Sin iniciar | — | — |
| P-5 | G-11 usuarios y auditoría para el admin | Sin iniciar | — | Recomendado junto con MFA de administradores (P32) |
