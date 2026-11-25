# Notificaciones: WhatsApp, correo y push

Estado: **Fase 7 en curso** (T7.1–T7.8 implementadas: §7–§13). ADR-0008, ADR-0009, ADR-0010 (push
aplazada por ADR-0019).

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

## 8. WhatsApp Cloud API (T7.2)

- **Cliente** `infrastructure/whatsapp`: `POST {WHATSAPP_API_BASE_URL}/{WHATSAPP_GRAPH_VERSION}/{WHATSAPP_PHONE_NUMBER_ID}/messages`
  con `Authorization: Bearer <token>`, cuerpo de plantilla (`messaging_product: whatsapp`, número sin
  `+`), respuesta → `messages[0].id` (wamid) como `provider_message_id`. Timeout 15 s.
- **Plantillas en el código** (`templates.go`): en T7.2 todos los tipos enviaban `hello_world`
  para probar la conexión; desde T7.3, las plantillas de la cita (§9).
- **Clasificación de errores de Meta:**

| Situación | Código guardado | ¿Reintenta? |
|---|---|---|
| Token inválido o vencido (190, 401) | `AUTH_FAILED` | Sí: al reemplazar el token salen los pendientes |
| Límite de envío (4, 80007, 130429, 131048, 131056, 429) | `RATE_LIMITED` | Sí |
| Falla de Meta (1, 2, 131000, 5xx) o sin conexión | `PROVIDER_ERROR` / `PROVIDER_UNREACHABLE` | Sí |
| Sin permiso (10, 200, 403) | `PERMISSION_DENIED` | Sí (es configuración) |
| No es destinatario registrado del número de prueba (131030) | `RECIPIENT_NOT_ALLOWED` | No |
| El número no puede recibir (131026) | `RECIPIENT_UNAVAILABLE` | No |
| Plantilla inexistente o mal formada (132xxx) | `TEMPLATE_INVALID` | No |
| Otro 4xx | `META_<código>` | No |

- **Secretos:** el token solo viaja en la cabecera. Los errores guardan el código numérico y el
  `fbtrace_id` de Meta, nunca su texto (puede citar el número) ni la URL. Al arrancar se registra
  solo qué proveedor se usa (`meta` o `simulated`), la versión y cuántos números hay en la lista
  blanca.
- **Configuración:** sin `WHATSAPP_ACCESS_TOKEN` → envío simulado. Con token: el identificador del
  número es obligatorio y numérico, y **fuera de producción `WHATSAPP_ALLOWLIST` es obligatorio**
  (la aplicación no arranca sin ella). En QA las credenciales están en `/opt/booking/qa/.env` y el
  compose solo se las pasa al **worker** (la API no envía).

## 9. Plantillas de la cita (T7.3)

Textos aprobados por el usuario el 2026-11-25 y creados por él en Meta (categoría **Utilidad**,
idioma `es`, variables numéricas). Los parámetros se llenan al enviar con el estado actual de la
cita (`scheduling.AppointmentState`: cliente, servicio del snapshot, barbero, negocio, zona y
motivo); la fecha va en la zona del negocio y en español: *"martes 2 de diciembre, 10:00 a. m."*.

| Tipo | Plantilla | Parámetros |
|---|---|---|
| `APPOINTMENT_CREATED` (el cliente reservó) | `cita_agendada` | cliente, negocio, servicio, barbero, fecha |
| `APPOINTMENT_CREATED_BY_BUSINESS` (P40) | `cita_agendada_negocio` + botón estático "Descargar la app" → `https://<dominio>/app` | cliente, negocio, servicio, barbero, fecha |
| `APPOINTMENT_CANCELLED` (por el negocio) | `cita_cancelada` | cliente, negocio, servicio, fecha, motivo |
| `APPOINTMENT_RESCHEDULED` | `cita_reprogramada` | cliente, servicio, negocio, nueva fecha, barbero |
| `APPOINTMENT_REASSIGNED` | `cita_cambio_barbero` | cliente, servicio, negocio, fecha, nuevo barbero |

- Cada parámetro se limpia para Meta: sin saltos de línea ni tabulaciones, sin espacios repetidos,
  máximo 120 caracteres, nunca vacío (`-`; motivo vacío → "Sin motivo indicado").
- **`GET /app`**: destino del botón. Con `APP_DOWNLOAD_URL` (https) redirige (302) a la tienda; sin
  ella muestra un aviso "la app estará disponible muy pronto". El enlace de la plantilla no cambia
  cuando la app se publique.
- **Botones Confirmar / Cancelar:** solo en el recordatorio (T7.4), decisión del usuario
  2026-11-25; necesitan el webhook (T7.5).
- Si una plantilla no existe o no está aprobada, Meta responde 132xxx → `FAILED TEMPLATE_INVALID`
  sin reintento.

## 10. Recordatorios (T7.4)

Decisiones del usuario (2026-11-25): texto y botones aprobados; **sin recordatorio si la cita está
confirmada** (las manuales nacen confirmadas); **si se agenda dentro de la ventana, el recordatorio
sale de inmediato**.

- **Cuándo se planea:** al grabarse un cambio que deja la cita `PENDING_CONFIRMATION` (creada,
  reprogramada o reasignada), si el negocio tiene los recordatorios activos
  (`BookingPolicy.RemindersEnabled`, `ReminderHoursBefore` 1–72, 24 por defecto). Se inserta un
  mensaje `APPOINTMENT_REMINDER` con `send_at = inicio − horas` (o el momento del cambio si ya
  pasó) y su job River con `ScheduledAt` en la misma transacción. Un aviso inmediato se encola sin
  hora (no depende del reloj del cambio).
- **Al vencer** se revalida: confirmada → `SKIPPED APPOINTMENT_CONFIRMED`; cancelada, movida o ya
  empezada → `SKIPPED` con su motivo. Un recordatorio viejo nunca se envía; el cambio posterior
  planea el suyo.
- **Plantilla** `recordatorio_cita` (cliente, servicio, negocio, fecha, barbero) con botones de
  respuesta rápida "Confirmar cita" / "Cancelar cita". Sus respuestas las procesa el webhook
  (T7.5), que agregará los códigos de un solo uso.
- Unicidad: `UNIQUE (event_id, kind, channel, recipient_role)` (migración 00021): el aviso y el
  recordatorio de un mismo cambio son mensajes distintos.

## 11. Webhook de WhatsApp (T7.5)

- **`GET /webhooks/whatsapp`**: verificación de Meta. Responde el `hub.challenge` (solo un número)
  si `hub.mode=subscribe` y `hub.verify_token` coincide (comparación en tiempo constante); si no,
  403. Sin configurar → 404.
- **`POST /webhooks/whatsapp`**: solo cuerpos firmados por Meta: `X-Hub-Signature-256 = sha256=` +
  HMAC-SHA256(App Secret, cuerpo crudo), comparado en tiempo constante; si no, 401
  `INVALID_SIGNATURE` (se registra sin el cuerpo). Máximo 1 MiB. Del campo `messages` se toman:
  estados `delivered`, `read`, `failed` (con su código) y respuestas de botón (`type=button`);
  lo demás se ignora. Cada elemento se inserta en `webhook_events` (`status:<wamid>:<estado>`,
  `message:<wamid>`): un reintento de Meta no se procesa dos veces. Se encola un job
  `notifications.process_webhook_item` y se responde 200 de inmediato; un error interno → 500
  (Meta reintenta).
- **Estados de entrega:** solo hacia adelante (`SENT → DELIVERED → READ`; `SENT/DELIVERED → FAILED`).
  Índice único en `provider_message_id`.
- **Botones del recordatorio:** al enviar el recordatorio se emiten dos códigos aleatorios de 32
  bytes (43 caracteres) como `payload` de "Confirmar cita" y "Cancelar cita"; se guarda solo su
  SHA-256 en `appointment_action_tokens`, con el número destino y vencimiento = inicio de la cita.
  Al responder, en una sola transacción: el código existe, el número que respondió es el destino
  del recordatorio, se bloquean los códigos de ese recordatorio (orden fijo), no está usado ni
  vencido, la cita sigue en la misma hora y barbero; luego se confirma o cancela **como el cliente**
  (`scheduling.CustomerReplies`, con su límite de cancelación) y se gastan los dos códigos. Un
  segundo toque, el otro botón o un número ajeno → "Este botón ya no es válido".
- **Respuesta al cliente** (mensaje de sesión, sin plantilla, dentro de las 24 h que abrió el
  cliente; job `notifications.send_reply`, misma lista blanca):

| Resultado | Texto |
|---|---|
| Confirmada | "¡Listo, {cliente}! Tu cita de {servicio} en {negocio} quedó confirmada para el {fecha}. Te esperamos." |
| Cancelada | "Tu cita de {servicio} en {negocio} del {fecha} quedó cancelada. Si quieres, puedes agendar otra desde la app." |
| Límite de cancelación | "Ya no es posible cancelar tu cita por aquí porque falta muy poco para la hora. Por favor comunícate con {negocio}." |
| Cita no activa | "Esta cita ya no está activa. Si quieres, puedes agendar una nueva desde la app." |
| Código inválido / usado / vencido / cita movida | "Este botón ya no es válido: ya respondiste o tu cita cambió. Revisa tu cita en la app o comunícate con el negocio." |

- **Configuración:** `WHATSAPP_APP_SECRET` (32 hex) y `WHATSAPP_WEBHOOK_VERIFY_TOKEN` (16–200),
  los dos o ninguno; en QA solo los recibe la API.

## 12. Correos de citas (T7.6, ADR-0019)

- **Canal `EMAIL`** junto a `WHATSAPP`: los mismos mensajes, jobs, reintentos, revalidación de la
  cita antes de enviar y lista de envíos de la cita (`GET …/notifications`, dirección enmascarada
  `l•••@gmail.com`). Interruptor `NOTIFICATIONS_EMAIL_ENABLED` (activo por defecto). SMTP del
  ambiente (Mailpit en QA; remitente `Booka QA`); proveedor de PROD pendiente (P13).
- **Destinatarios** (decisiones del usuario, 2026-11-25):

| Cambio | Cliente (WhatsApp) | Cliente (correo) | Barbero (correo) |
|---|---|---|---|
| Agendada por el cliente | ✅ | ✅ | ✅ "Nueva cita" |
| Agendada por el negocio | ✅ (con enlace de la app) | ✅ | — |
| Reprogramada por el cliente | ✅ | ✅ | ✅ "Cita reprogramada" |
| Reprogramada o reasignada por el negocio | ✅ | ✅ | — |
| Cancelada por el negocio | ✅ | ✅ (con motivo) | — |
| Cancelada por el cliente | — | ✅ comprobante (`APPOINTMENT_CANCELLED_BY_CUSTOMER`) | ✅ "Cita cancelada" |
| Recordatorio (cita pendiente) | ✅ con botones | ✅ (sin botones hasta T7.7) | — |

  Al barbero solo se le avisa lo que hizo el cliente.
- **Dirección:** se resuelve al enviar y se guarda en el mensaje. Cliente: el correo que registró
  el negocio en su ficha; si no hay, el de su cuenta. Barbero: el de su cuenta. Cuenta inexistente
  o deshabilitada, o sin correo → `SKIPPED NO_RECIPIENT`.
- **Contenido** (HTML con versión de texto, `multipart/alternative`; todo valor escapado por
  `html/template`): marca Booka, encabezado morado con el negocio, título, estado de la cita
  (pendiente de confirmar, confirmada, cancelada…), servicio, fecha y hora en la zona del negocio,
  barbero, dirección con enlace a Google Maps (coordenadas o dirección), motivo de cancelación,
  políticas del negocio (límites de cancelación y reprogramación) y contacto (teléfono y enlace de
  WhatsApp); pie "Agendado con Booka" y "Descarga la app" si `APP_DOWNLOAD_URL` está configurado.
  El correo del barbero lleva cliente, servicio, fecha y hora (sin teléfono del cliente ni
  políticas).

| Tipo | Asunto |
|---|---|
| Agendada (cliente o negocio) | "Tu cita en {negocio} está agendada" |
| Cancelada (por el negocio o por el cliente) | "Tu cita en {negocio} fue cancelada" |
| Reprogramada | "Tu cita en {negocio} cambió de hora" |
| Cambio de especialista | "Tu cita en {negocio} tiene otro especialista" |
| Recordatorio | "Recordatorio: tu cita en {negocio} es el {fecha}" |
| Barbero: nueva / cancelada / reprogramada | "Nueva cita: {cliente}, {fecha}" / "Cita cancelada: …" / "Cita reprogramada: …" |

- **Errores:** dirección mal formada o respuesta 5xx del servidor SMTP → `FAILED` sin reintento
  (`INVALID_EMAIL`, `SMTP_5xx`); lo demás se reintenta (`SMTP_ERROR`).

## 13. Acciones desde el correo (T7.7, ADR-0019)

- **Botones del correo al cliente** sobre una cita activa (agendada, reprogramada, cambio de
  especialista, recordatorio): **Confirmar cita** (solo si está pendiente de confirmar),
  **Reagendar** y **Cancelar cita**. No los llevan los correos de cancelación ni los del
  especialista.
- **Códigos de un solo uso** (`appointment_action_tokens`, acción `RESCHEDULE` nueva, migración
  00024): 32 bytes aleatorios, solo su SHA-256 guardado, ligados al mensaje y a la dirección,
  vencen al inicio de la cita. Quien tiene el código actúa como el cliente, con sus reglas
  (límites de cancelación y reprogramación).
- **Dos toques** (decisión del usuario): `GET /citas/confirmar|cancelar|reagendar?token=…` solo
  muestra la cita y un botón (los revisores de enlaces de Outlook, antivirus o pasarelas abren los
  enlaces); el botón hace `POST` y ahí ocurre el cambio. Páginas HTML con la marca Booka, sin
  scripts; CSP `default-src 'none'; style-src 'unsafe-inline'; form-action 'self'`,
  `Referrer-Policy: no-referrer`, `no-store`, `noindex`; límite estricto por cliente (el de
  autenticación). Toda página responde 200 y dice qué pasó (incluido "Este enlace ya no es
  válido").
- **Reglas:** confirmar gasta solo su enlace (luego aún se puede cancelar desde el mismo correo);
  cancelar o cambiar la hora gasta todos los enlaces del correo; un enlace de otra acción, usado,
  vencido o de una cita que cambió desde el correo → inválido; los enlaces de un correo se
  bloquean juntos (orden fijo), así que dos usados a la vez aplican uno solo; horario tomado o
  límite pasado → el enlace sigue válido.
- **Reagendar:** si `APPOINTMENT_RESCHEDULE_URL` está configurado, el botón abre esa página del
  frontend con `?token=`; si no, la página de la API indica usar la app. API para el frontend
  (pública, el código es la credencial):
  - `POST /api/v1/appointment-links/lookup {token, action}` → `outcome` (null si se puede usar),
    datos de la cita (`businessSlug`, `serviceId`, `professionalId`, `startAt`, `timeZone`,
    límites) para consultar `GET /api/v1/businesses/{slug}/availability`;
  - `POST /api/v1/appointment-links/apply {token, action, startAt?}` → `CONFIRMED`, `CANCELLED`,
    `RESCHEDULED`, `CUTOFF_PASSED`, `SLOT_UNAVAILABLE`, `RESCHEDULE_LIMIT_REACHED` (T7.8),
    `NOT_ACTIVE` o `INVALID`.
- **Límite de reprogramaciones (T7.8):** el correo no lleva **Reagendar** si el cliente ya usó
  los cambios que permite el negocio, y las políticas del correo dicen cuántos le quedan; un
  enlace anterior responde `RESCHEDULE_LIMIT_REACHED` ("Ya no puedes cambiar la hora por aquí")
  sin gastarse: el mismo correo aún sirve para confirmar o cancelar. `lookup` devuelve
  `maxCustomerReschedules` y `customerReschedules`.
- **Configuración:** `PUBLIC_API_URL` (origen público de la API; https en QA/PROD; en QA sale de
  `SITE_DOMAIN`) y `APPOINTMENT_RESCHEDULE_URL` (opcional, https).