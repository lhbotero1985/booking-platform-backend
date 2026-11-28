# ADR-0021: Reseñas y calificaciones de negocios y profesionales

- Estado: Aceptado (decisiones del usuario, 2026-11-28) · Fecha: 2026-11-28
- Cubre la brecha G-18 del frontend (reseñas y calificaciones; los favoritos siguen fuera).
- Fuera de fase: el roadmap dejaba las reseñas fuera del MVP (riesgo R8); el usuario las pidió.

## Contexto
El usuario pidió que los clientes puedan evaluar a los negocios y a sus profesionales. Para
reservar en línea el cliente ya necesita una cuenta con correo verificado, así que cada reseña
puede quedar ligada a una cita real. Las citas que el personal registra a mano pueden no tener
cuenta de cliente.

## Decisión
1. **Quién y cuándo:** solo el cliente (con cuenta) de una cita **COMPLETED**, una reseña por
   cita, hasta **30 días** después de que la cita termina. Puede cambiarla en ese plazo y
   borrarla cuando quiera. Una cita cancelada o marcada como no asistida no se califica.
2. **Qué califica:** el negocio (1 a 5) y el profesional que lo atendió (1 a 5) por separado, más
   un comentario opcional de hasta 1000 caracteres.
3. **El negocio:** el OWNER responde en público (y cambia o quita su respuesta), pero no puede
   ocultar ni borrar reseñas. El personal las lee todas (también las ocultas, marcadas); un
   PROFESSIONAL solo las suyas.
4. **Moderación:** solo un administrador de la plataforma oculta una reseña abusiva, con motivo.
   La fila guarda quién la ocultó, cuándo y por qué. Una reseña oculta sale de la lista pública y
   de los promedios.
5. **Lo público:** promedio (un decimal) y número de reseñas visibles en `PublicBusiness` (detalle
   y búsqueda) y en cada profesional del catálogo; lista paginada de reseñas del negocio. El
   cliente aparece como nombre y una inicial («Ana M.»); nunca su cuenta, correo ni teléfono.
6. **Invitación:** al quedar COMPLETED la cita, el cliente con cuenta recibe un correo
   (`APPOINTMENT_REVIEW_REQUEST`, sin WhatsApp) con «Calificar» hacia la página del frontend
   `APPOINTMENT_REVIEW_URL` (recibe `?appointmentId=`; el cliente inicia sesión allí). Sin esa
   URL, el correo invita a calificar desde «Mis citas». Respeta las preferencias de correo del
   cliente y no sale si la cita dejó de estar completada.
7. **Dónde vive:** en el módulo `scheduling`, dueño de las citas (sin módulo nuevo). Publica el
   contrato `scheduling.Ratings`; las vistas públicas de `businesses` y `catalog` lo usan, por lo
   que sus fronteras (depguard) permiten ahora `scheduling/contract`.
8. **Concurrencia:** escribir la reseña bloquea la fila de la cita, así dos envíos simultáneos se
   ordenan (el primero crea, el resto cambia). La restricción `UNIQUE (appointment_id)` es la red
   de seguridad (verificado con 40 envíos simultáneos en PostgreSQL).

## Consecuencias
- Migración 00030: tabla `appointment_reviews` (con `CHECK` de estrellas, largos y coherencia de
  respuesta y ocultamiento) y el tipo de mensaje `APPOINTMENT_REVIEW_REQUEST`.
- Nueva variable opcional `APPOINTMENT_REVIEW_URL` (https, sin query).
- El texto del correo es una propuesta (P45): el usuario puede ajustarlo.
- Los promedios se calculan al leer (sin columnas denormalizadas): suficiente para el volumen del
  MVP; si crece, se pueden materializar.
