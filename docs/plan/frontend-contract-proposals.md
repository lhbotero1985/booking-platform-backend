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
| P-9 | G-27 horas libres para el personal | ✅ En `qa`, E2E 241/241 (2026-11-26) | PR #76 | `GET /business/{id}/availability`: sin anticipación mínima ni requisito de publicación; **respeta el límite de días** (decisión del usuario: igual que la reserva del personal); `excludeAppointmentId` opcional |
| P-10 | G-28 citas de un cliente | ✅ En `qa`, E2E 241/241 (2026-11-26) | PR #76 | `GET /business/{id}/customers/{customerId}/appointments?scope=upcoming\|past` (decisión del usuario: próximas ascendentes, pasadas de la más reciente); cubre también la segunda fila de P-4 |
| P-11 | G-29 un dueño crea más negocios | ✅ Implementada (2026-11-27) | `feature/p11-multiple-businesses` | Decisión del usuario: **hasta 2 negocios propios durante la demo** (`OWNED_BUSINESS_LIMIT`), cada uno con su demo; el tercero → 409 `OWNED_BUSINESS_LIMIT`; `Idempotency-Key` opcional (otra petición con la misma clave → 422 `IDEMPOTENCY_KEY_REUSED`) |
| P-12 | G-12 estadísticas del negocio y consolidadas | ✅ Implementada (2026-11-27) | `feature/p11-multiple-businesses` (un solo PR con P-11, decisión del usuario) | `GET /business/{id}/stats` y `GET /me/stats` (`from`, `to`, `groupBy=day\|month\|year`, `professionalId`); 400 `INVALID_RANGE`. Decisiones del usuario: ocupación con el horario **actual** (aproximada si cambió); las ausencias del cliente (`NO_SHOW`) no cuentan como minutos ocupados; profesionales inactivos con el horario que dejaron, desde que se creó su perfil; ingresos con el precio al agendar; `/me/stats` en cero sin negocios propios; sin acceso del admin. `customersServed` no se suma entre profesionales ni servicios |
| — | P-7, P-8 (frontend) | Sin revisar | — | Están en el documento del frontend; no autorizadas en el backend |

P-9 y P-10 se recibieron por el chat (2026-11-26) antes de estar en el documento del frontend;
el frontend debe agregarlas a `docs/BACKEND_CONTRACT_PROPOSALS.md`.
