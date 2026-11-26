# Cierre de la Fase 7 — Notificaciones

Fecha: 2026-11-25 · Estado: **✅ Aprobada por el usuario (2026-11-25)**

## 1. Entregables

### Lo que hace hoy la plataforma

1. **Cada cambio de una cita avisa a quien corresponde**, en la misma transacción del cambio
   (nunca un aviso de un cambio que no ocurrió) y una sola vez:
   - al **cliente**, por WhatsApp y por correo: agendada, reprogramada, cambio de especialista,
     cancelada (con motivo), y el comprobante cuando él mismo cancela;
   - al **especialista**, por correo, solo lo que hizo el cliente: nueva cita, reprogramada,
     cancelada.
2. **Recordatorio** antes de la cita (horas configurables por negocio) cada vez que la cita queda
   pendiente de confirmar. Al enviarse se revisa la cita: si se confirmó, se movió, se canceló o
   ya pasó, no sale.
3. **WhatsApp (Cloud API de Meta)** con plantillas; botones **Confirmar / Cancelar** en el
   recordatorio. El webhook recibe los botones y los estados de entrega (enviado, entregado,
   leído, fallido). Sin credenciales se usa un envío simulado; en QA solo números de prueba.
4. **Correo con la marca Booka** (HTML y texto): datos de la cita, dirección con mapa, contacto
   del negocio y sus políticas. Botones **Confirmar cita, Reagendar y Cancelar cita** en **dos
   toques**: abrir el enlace solo muestra la cita (los revisores de correo de Outlook o los
   antivirus abren todos los enlaces); el cambio ocurre al tocar el botón de la página.
5. **Límite de reprogramaciones por cita** configurable por negocio (2 por defecto); solo cuentan
   los cambios del cliente.
6. **Preferencias**: el cliente elige WhatsApp y/o correo (al menos uno) y si quiere
   recordatorios; el especialista o el dueño apagan los correos del especialista.
7. **Control de costos (P16)**: tope mensual de WhatsApp por negocio que fija el administrador de
   Booka (500 por defecto) y tope diario global opcional; al llegar al tope sale solo el correo.
8. **Historial de mensajes** de cada cita para el personal: canal, destinatario enmascarado,
   estado y motivo cuando no se envió.

Validado en QA de punta a punta con el script E2E: **231/231** tras T7.9.

### Tareas

| Tarea | Entregable | Estado | Evidencia |
|---|---|---|---|
| T7.1 | Base: mensajes planificados con cada cambio, cola River, historial | ✅ | PR #56 |
| T7.2 | Envío por la Cloud API de WhatsApp, errores permanentes y reintentos | ✅ | PR #57 |
| T7.3 | Plantillas de los avisos y página de descarga de la app | ✅ | PR #58 |
| T7.4 | Recordatorios programados con cada cambio pendiente | ✅ | PR #59 |
| T7.5 | Webhook: estados de entrega y botones del recordatorio | ✅ | PR #60 |
| T7.6 | Correos de citas al cliente y al especialista (Booka) | ✅ | PR #61, E2E 209/209 |
| T7.7 | Acciones desde el correo en dos toques; API para la página de reagendar | ✅ | PR #66 y #67, E2E 214/214 |
| T7.8 | Límite de reprogramaciones por cita | ✅ | PR #68 y #69, E2E 218/218 |
| T7.9 | Preferencias de notificación y topes de WhatsApp | ✅ | PR #70, E2E 231/231 |
| T7.10 | Seguridad del webhook y de los enlaces, este documento | ✅ | `feature/t7.10-phase7-closure` |
| — | Push (Expo): **aplazada** a la fase de las apps (ADR-0019) | Aplazada | Decisión del usuario |

Módulo nuevo: `notifications`. Migraciones 00019–00026. API: **98 operaciones, todas en
`api/openapi.yaml`** (81 rutas al cerrar la Fase 6).

## 2. Pruebas

| Tipo | Estado | Evidencia (`docs/testing/test-results.md`) |
|---|---|---|
| Unitarias | ✅ | Cobertura: notifications dominio 100 %, aplicación 90,9 %, correo 96,0 %, WhatsApp 94,9 %, worker 94,4 %; scheduling dominio 100 %, aplicación 85,6 %; negocios aplicación 90,3 % |
| Integración (PostgreSQL real) | ✅ | Mensajes en la transacción del cambio, recordatorios, webhook, enlaces, preferencias y topes con SQL real |
| Concurrencia | ✅ | Botones Confirmar ∥ Cancelar del mismo recordatorio y enlaces Cancelar ∥ Reagendar del mismo correo: **exactamente uno se aplica**; dos reprogramaciones del cliente con un cambio disponible: una |
| Contrato OpenAPI | ✅ | 98 operaciones ↔ rutas en ambas direcciones; cada operación nueva validada con respuestas reales |
| Seguridad (`tests/security`) | ✅ | Aislamiento entre negocios en **42 operaciones** (27 con ids ajenos); matriz de credenciales inválidas en **73 operaciones autenticadas**; enlaces y webhook atacados de punta a punta (§3) |
| Mutaciones | ✅ | Detectadas todas, entre ellas: enlaces sin bloqueo (cancelada **y** movida), confirmar gastando todos los enlaces, límite con `>` en vez de `>=`, contador no guardado, envío sin filtro de preferencias, tope con `>`, código vencido con respuesta distinta |
| E2E en QA (HTTPS público, Mailpit) | ✅ | **231/231**; incluye los enlaces reales del correo en Mailpit |
| Estático y dependencias | ✅ | golangci-lint (depguard por módulo), sqlc vet/diff, govulncheck, gitleaks, Trivy, shellcheck |
| **Mensajes reales de WhatsApp** | ⏳ | Las 6 plantillas de Booka siguen **PENDING** en Meta (consultado 2026-11-25); `hello_world` sí llegó al número de prueba (T7.2). Se prueba cuando Meta las apruebe |

## 3. Checklist de seguridad de la fase

| Control | Verificación |
|---|---|
| Webhook solo de Meta: firma HMAC-SHA256 del cuerpo tal como llegó, comparación en tiempo constante | `TestWebhookRejectsForgeries`: sin firma, otra clave, cuerpo cambiado tras firmar, cabecera sha1 y firma mal formada → rechazados y nada encolado |
| Verificación del webhook solo con el verify token | Token incorrecto → 403 sin devolver el reto |
| Meta reintenta: cada evento se procesa una vez | `TestReceiveWebhookOnce`, `TestWhatsAppButtonsOnPostgres` |
| Un botón solo vale desde el número al que se envió | Respuesta de otro número → "ya no es válido", sin cambios |
| Códigos de un solo uso: 32 bytes aleatorios, solo su SHA-256 guardado, vencen al inicio de la cita | Pruebas de dominio e integración (no se guarda el código crudo) |
| **Un enlace inutilizable no da pistas**: desconocido, de otra acción, gastado o vencido responden la misma página y el mismo JSON | `TestEmailLinksGiveNothingAway` (mutación: vencido distinto → detectado) |
| Abrir un enlace nunca cambia la cita | Cinco aperturas de cada enlace: sin cambios |
| Sin XSS: lo escrito en la URL no se refleja; datos escapados; CSP `default-src 'none'` | `TestEmailLinksGiveNothingAway`, `TestLinkPages` |
| Adivinar códigos está limitado por cliente | `TestEmailLinksAreRateLimited` (429 con `Retry-After`) |
| Los códigos y teléfonos no se registran en los logs | La prueba revisa los logs; destinatarios enmascarados en el historial |
| Preferencias: el especialista solo lo cambia él o el dueño; el tope solo el administrador | Pruebas de aplicación y contrato (403/404); autorización antes de validar (detectado y corregido en T7.9) |
| Costo acotado | Topes mensual y diario; lista blanca de números en QA |
| Secretos de Meta solo en el `.env` del servidor | gitleaks; nunca impresos (la consulta de plantillas solo muestra nombre y estado) |

**CSRF en las páginas de los enlaces:** no aplica: el código del enlace es la credencial y va en
el formulario; un tercero sin el código no puede provocar el cambio.

## 4. Decisiones del usuario incorporadas

- Push aplazada a la fase de las apps; correo con la marca **Booka** (identidad propia, no copia
  de la competencia).
- Acciones desde el correo en **dos toques** (también confirmar).
- El cliente recibe ambos canales, configurables, al menos uno obligatorio; el especialista
  recibe solo lo que hizo el cliente; término "especialista".
- El cliente recibe comprobante por correo cuando él cancela.
- Límite de reprogramaciones: 2 por defecto, solo cuentan los del cliente.
- Preferencias del cliente por cuenta; correos del especialista: él o el dueño; tope de WhatsApp
  del administrador, 500 por defecto.

## 5. Riesgos aceptados y pendientes

| Tema | Estado |
|---|---|
| Plantillas de WhatsApp pendientes en Meta; verificación del negocio (P17) | Externo. Prueba real de mensajes y botones cuando Meta apruebe |
| Plantilla `cita_cambio_barbero` dice "barbero" (P42) | Crear una plantilla nueva con "especialista" cuando Meta apruebe las actuales |
| Topes de WhatsApp aproximados (sin bloqueo) | Aceptado: se pueden pasar por los mensajes en curso en el mismo instante |
| Proveedor de correo de producción (P13) | Fase de despliegue a producción; en QA, Mailpit |
| `APP_DOWNLOAD_URL` y `APPOINTMENT_RESCHEDULE_URL` | Se configuran cuando existan la app y la página de reagendar del frontend |
| Textos legales de los mensajes (P41) | Pendiente del usuario |
| Mensajes y tokens no se purgan | Purga programada en la Fase 12 |
| Handlers HTTP de notificaciones con 59 % de cobertura en su paquete | Se ejercitan además con contrato, seguridad y E2E |

## 6. Aprobación

- [x] Usuario aprueba el cierre de la Fase 7 (2026-11-25).
