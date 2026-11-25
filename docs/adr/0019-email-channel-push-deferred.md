# ADR-0019: Correo como canal de citas; push aplazada

- Estado: Aceptado (decisión del usuario, 2026-11-25) · Fecha: 2026-11-25
- Modifica ADR-0010 (push con Expo): la push no se construye en la Fase 7; se retoma en la fase de
  las apps móviles. El resto de ADR-0010 sigue vigente para entonces.

## Contexto
La Fase 7 preveía WhatsApp al cliente y push al barbero. Las apps aún no existen (el frontend
empieza en otro repositorio), así que la push no podía validarse. El usuario prefiere que el
cliente reciba también un correo con los datos de la cita y pueda confirmarla, cancelarla o
reagendarla desde allí, y que el barbero reciba sus avisos por correo.

## Decisión
- **Canal `EMAIL`** en `notifications`, junto a `WHATSAPP`: mismos eventos, mismo registro de
  mensajes, misma revalidación de la cita antes de enviar y reintentos por River. SMTP
  (Mailpit en QA; proveedor de PROD pendiente, P13). Correo en HTML con versión de texto, marca
  **Booka**, identidad propia.
- **Cliente:** correo y WhatsApp. Cada cliente elige sus canales con al menos uno activo (T7.9).
  Sin correo registrado → el correo se omite (`NO_RECIPIENT`).
- **Barbero:** correo con sus reservas, cancelaciones y cambios, que puede apagar (T7.9).
- **Acciones desde el correo en dos toques:** el enlace abre una página y la acción ocurre solo
  al tocar su botón (POST). Los revisores de enlaces (Outlook Safe Links, antivirus, pasarelas
  corporativas) abren los enlaces de los correos; un enlace que actuara con solo abrirse podría
  confirmar o cancelar citas sin la persona. Códigos de un solo uso como en T7.5 (solo el hash
  guardado, vencen al inicio de la cita). Reagendar requiere elegir un horario: el backend expone
  los servicios con el código; la pantalla la hace el frontend.
- **Límite de reprogramaciones** por cita, configurable por negocio (T7.8).

## Consecuencias
- El barbero se entera por correo, no al instante, hasta que exista la app con push.
- Las citas de clientes sin correo solo avisan por WhatsApp.
- PROD necesita un proveedor de correo con SPF/DKIM en el dominio (P13) antes del lanzamiento.
