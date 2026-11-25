# Notificaciones: WhatsApp y push

Estado: **Fase 7 en curso** (T7.1 implementada: §7). ADR-0008, ADR-0009, ADR-0010.

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
| `AppointmentReassigned` (otro barbero) | Push | WhatsApp + push (el cliente eligió el barbero) |
| Iniciada, completada, no asistió | — | — |

## 3. WhatsApp (Meta Cloud API)

- **Número central de la plataforma** (ADR-0009). Cada plantilla incluye el nombre del negocio
  como variable: "{{negocio}}: tu cita de {{servicio}} es el {{fecha}} a las {{hora}}".
- Mensajes iniciados por la empresa → **solo plantillas aprobadas** (categoría *utility*).
  Plantillas versionadas en código (nombre + idioma + parámetros) y registro de cuáles están
  aprobadas por ambiente.
- **Consentimiento** (decisión del usuario, 2026-11-24): **agendar una cita es la aceptación del
  cliente a recibir WhatsApp sobre esa cita**, tanto si reserva en la app como si el barbero lo
  agenda a mano. La evidencia es el evento `CREATED` de la cita (quién la creó, con qué rol y
  cuándo). Solo mensajes de la cita (utility), nunca publicidad. El cliente podrá apagar los
  recordatorios (T7.7). Ver P41 (revisión legal antes de PROD).
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

## 7. Implementación (T7.1)

- **Módulo `notifications`** (dominio, aplicación, `infrastructure/{postgres,worker,simulated}`,
  `interfaces/http`). Lee a `scheduling` solo por su contrato.
- **Encolado en la transacción del cambio:** `scheduling` avisa cada evento grabado a un
  `scheduling.EventListener` (contrato) dentro de su transacción; `notifications` lo implementa:
  calcula los mensajes (`domain.Plan`), los inserta en `notification_messages` y encola un job
  `notifications.send_message` por mensaje con `InsertTx`. Si el cambio se deshace, el mensaje y el
  job también (probado con PostgreSQL real y con una mutación).
- **Una sola vez:** `UNIQUE (event_id, channel, recipient_role)`; el job es único por mensaje; el
  envío solo actúa sobre mensajes `PENDING` y el `UPDATE` final exige `status = 'PENDING'`.
- **Revalidación al enviar** (`domain.Stale`): la cita ya empezó → `APPOINTMENT_PASSED`; salvo el
  aviso de cancelación, si la cita se canceló → `APPOINTMENT_CANCELLED` y si cambió de hora o de
  barbero → `APPOINTMENT_CHANGED` (el cambio posterior tiene su propio mensaje). Una confirmación
  no invalida el mensaje.
- **Interruptor por ambiente:** `WHATSAPP_ENABLED` (falso por defecto: DEV) y `WHATSAPP_ALLOWLIST`
  (E.164; si se define, solo esos números). QA: encendido con el envío simulado hasta tener las
  credenciales de Meta (T7.2).
- **Envío simulado:** registra el envío en el log con el número enmascarado (`+57••••••4567`) y un
  id sintético; nada sale del servidor.
- **Reintentos:** River, 8 intentos con backoff; error permanente (`DeliveryError.Permanent`) o
  último intento → `FAILED`; cada intento guarda su código de error.
- **Vista del personal:** `GET /api/v1/business/{businessId}/appointments/{appointmentId}/notifications`
  con el mismo acceso que el historial (responsable todas; barbero solo su agenda, 403; otro
  negocio 404). El número se muestra enmascarado.