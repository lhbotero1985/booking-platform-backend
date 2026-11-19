# Notificaciones: WhatsApp y push

Estado: **Propuesta — Fase 0.** ADR-0008, ADR-0009, ADR-0010.

## 1. Principios

- **Una reserva nunca falla porque falle una notificación.** Los envíos son jobs River encolados
  en la misma transacción que el cambio de la cita (`InsertTx`) y se procesan en `cmd/worker`.
- Toda notificación es **idempotente**: clave `appointmentId:evento:version:canal`, única en
  `notification_messages` y en River (`UniqueOpts`).
- Toda notificación se re-evalúa al ejecutarse: si la cita cambió (versión distinta, cancelada,
  ya pasó), el job termina como `SKIPPED` sin enviar.

## 2. Eventos y canales

| Evento de dominio | Profesional | Cliente |
|---|---|---|
| `AppointmentBooked` | Push | WhatsApp (plantilla de creación) + push |
| Recordatorio (`inicio − 24 h`) | — | WhatsApp con botones Confirmar / Cancelar + push |
| `AppointmentConfirmed` | Push | — (confirmación ya visible en la app) |
| `AppointmentCancelled` | Push | WhatsApp + push (si no canceló el propio cliente) |
| `AppointmentRescheduled` | Push | WhatsApp + push |

## 3. WhatsApp (Meta Cloud API)

- **Número central de la plataforma** (ADR-0009). Cada plantilla incluye el nombre del negocio
  como variable: "{{negocio}}: tu cita de {{servicio}} es el {{fecha}} a las {{hora}}".
- Mensajes iniciados por la empresa → **solo plantillas aprobadas** (categoría *utility*).
  Plantillas versionadas en código (nombre + idioma + parámetros) y registro de cuáles están
  aprobadas por ambiente.
- **Consentimiento**: opt-in explícito del cliente (casilla en la app / registro verbal en reserva
  manual, con fuente registrada). Sin consentimiento vigente → mensaje `SKIPPED`.
- **Botones interactivos**: plantilla con botones *quick reply* cuyo `payload` es un token opaco
  de un solo uso (`appointment_action_tokens`), no el ID de la cita. Al recibir la respuesta:
  validar firma del webhook → token existe, no usado, no expirado, versión de cita vigente, y el
  `wa_id` remitente coincide con el teléfono destino → ejecutar `ConfirmAppointment` /
  `CancelAppointment` con actor `CUSTOMER` vía WhatsApp → responder con mensaje de sesión.
- **Webhooks** (`POST /webhooks/whatsapp`):
  1. Verificación `GET` con `hub.verify_token` (secreto por ambiente).
  2. Validar `X-Hub-Signature-256` (HMAC-SHA256 con App Secret) sobre el cuerpo crudo.
  3. Insertar en `webhook_events (provider, event_id)` — duplicado → 200 sin reprocesar.
  4. Encolar job `ProcessWhatsAppWebhook` y responder 200 rápido (< 1 s).
  5. Estados de entrega (`sent`, `delivered`, `read`, `failed`) actualizan `notification_messages`.
- **Reintentos**: River con backoff exponencial; errores 4xx no recuperables (número inválido,
  plantilla rechazada) → `FAILED` sin reintento; 429/5xx → reintento (máx. 8 intentos).
- **Control de costos**: solo eventos de la tabla anterior; sin mensajes de marketing; tope diario
  configurable por negocio y global; métrica de mensajes por negocio/mes; kill switch por
  ambiente (`WHATSAPP_ENABLED=false` en DEV; en QA solo números de prueba en lista blanca).

## 4. Push (Expo Push Notifications — ADR-0010)

- Las apps registran el `ExpoPushToken` tras login (`POST /me/devices`), lo renuevan al abrir la
  app y lo eliminan al cerrar sesión (`DELETE /me/devices/{id}`).
- Envío por lotes a la API de Expo con *access token* de Expo (secreto por ambiente); procesamiento
  de *push receipts* en un job diferido; `DeviceNotRegistered` → revocar token.
- Payload sin datos sensibles: texto breve + `appointmentId` para navegación.

## 5. Jobs River

| Job | Disparo | Unicidad | Notas |
|---|---|---|---|
| `SendWhatsAppMessage` | Evento de cita | `idempotency_key` | Reintentos con backoff |
| `SendPushNotification` | Evento de cita | `idempotency_key` | Lotes a Expo |
| `AppointmentReminder` | Programado `inicio − 24 h` | `appointmentId + version` | Al ejecutar, verifica estado/versión; si obsoleto → descarta. Al cancelar/reprogramar, además se cancela el job anterior (limpieza). |
| `ProcessWhatsAppWebhook` | Webhook recibido | `event_id` | |
| `CheckPushReceipts` | 15 min tras envío | por lote | |
| `CleanupExpired` | Periódico (diario) | — | Sesiones, tokens, idempotency keys |

Colas separadas (`notifications`, `maintenance`) para que un proveedor lento no retrase la
limpieza. Los resultados quedan en `notification_messages` y en las tablas de River.

## 6. Preferencias

El cliente puede desactivar recordatorios por canal. Las notificaciones transaccionales críticas
(cancelación por parte del negocio) se envían por al menos un canal disponible.
