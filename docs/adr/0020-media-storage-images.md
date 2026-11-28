# ADR-0020: Imágenes de negocios y profesionales en el disco del servidor

- Estado: Aceptado (decisiones del usuario, 2026-11-27) · Fecha: 2026-11-27
- Cubre la brecha G-09 del frontend (logo, portada y fotos de profesionales).

## Contexto
El usuario pidió que un negocio tenga **logo e imagen de portada** y que cada profesional pueda
subir **hasta 10 imágenes de sus mejores trabajos**. Hasta ahora la API no guardaba archivos. Las
opciones eran el disco del servidor (sin costo) o un almacenamiento externo compatible con S3
(DigitalOcean Spaces, unos USD 5 al mes, recurso pago que solo el usuario puede crear).

## Decisión
1. **Almacén:** el disco del servidor, en un volumen Docker `media` montado en `/data/media`
   (`MEDIA_DIR`) solo en la API; el resto del contenedor sigue de solo lectura. El código usa la
   interfaz `media.Store` (`internal/platform/media`), así que **DigitalOcean Spaces** puede
   reemplazarla antes de PROD sin tocar los módulos (decisión pendiente, P44).
2. **Servir:** la API responde `GET /media/{key...}` (público, inmutable: una subida nueva lleva una
   clave nueva; `Cache-Control` de un año; `Cross-Origin-Resource-Policy: cross-origin` para que el
   portal, en otro dominio, pueda mostrarlas). Las URL salen de `PUBLIC_API_URL`.
3. **Seguridad de cada subida:** `multipart/form-data` (campo `image`, 5 MiB; el límite general de
   1 MiB sigue para el resto). Solo JPEG, PNG y WebP **por contenido**; tope de 24 megapíxeles y
   12000 px por lado (bombas de descompresión); la imagen se **decodifica y codifica de nuevo**
   (se pierden los metadatos, como la ubicación GPS, y un archivo disfrazado no sobrevive); se
   corrige la orientación EXIF y se reduce (logo 512 px, portada y portafolio 1600 px); JPEG, o PNG
   si tiene transparencia. La clave la genera el servidor
   (`carpeta/<negocio>/<uuid>.jpg|png`) y el esquema la valida con un `CHECK`. Como mucho dos
   imágenes se procesan a la vez (memoria del servidor).
4. **Permisos:** logo y portada, solo el OWNER. Portafolio: el profesional el suyo y el OWNER el de
   cualquiera (decisión del usuario). La autorización va antes de leer el archivo: un negocio ajeno
   es 404 sea cual sea el cuerpo. Todo queda auditado.
5. **Límite de 10:** se cuenta con la fila del profesional bloqueada, así dos subidas simultáneas
   no lo superan (verificado con 12 subidas simultáneas en PostgreSQL).
6. **Consistencia archivo–base de datos:** el archivo se escribe antes de la transacción; si la
   transacción falla, se borra. Al reemplazar o borrar, el archivo viejo se borra después del
   commit; si eso falla queda un archivo huérfano, nunca una imagen rota.

## Consecuencias
- Nueva dependencia `golang.org/x/image` (decodificar WebP y escalar), del proyecto Go.
- Migración 00029: `businesses.logo_key/cover_key` y `professional_portfolio_images`.
- El volumen `media` **no está en el respaldo diario** (`backup.sh` solo respalda PostgreSQL): en QA
  son datos de prueba; antes de PROD hay que decidir el respaldo o pasar a Spaces (P44).
- Comparte el disco del Droplet (70 GB en QA): 10 imágenes de ~300 KB por profesional son pocos MB
  por negocio.
