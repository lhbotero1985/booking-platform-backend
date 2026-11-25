# Preguntas pendientes

Cada pregunta tiene una **propuesta por defecto**. Si el usuario no indica otra cosa al aprobar
la Fase 0, se adopta la propuesta.

## Bloqueantes para Fase 1

| # | Pregunta | Propuesta por defecto |
|---|---|---|
| P1 | ~~¿Polyrepo o monorepo?~~ | ✅ **Respondida 2026-11-18:** workspace local único, dos repos en GitHub (backend y frontend) — ADR-0016. |
| P2 | ~~¿Privado o público?~~ | ✅ **Respondida: privado.** Pendiente: usuario/organización de GitHub (define la ruta del módulo Go), plan Free o Pro/Team (protección de ramas), URL del repo vacío y autorización para el push. |
| P22 | ¿La infraestructura QA/PROD va en el repo backend (`deploy/`)? | Sí (ADR-0016); confirmar en Fase 3. |
| P3 | ~~¿Instalar Go?~~ | ✅ **Respondida: autorizado.** |
| P4 | ¿Actualizar Git local (2.30.1, de 2021)? | Sí, recomendado. |
| P21 | ~~Aprobación de la Fase 0~~ | ✅ Aprobada 2026-11-18. |

## Bloqueantes para Fases 4–7 (pueden responderse más tarde)

| # | Pregunta | Propuesta por defecto |
|---|---|---|
| P5 | País(es) y moneda(s) del piloto | ✅ Decidido 2026-11-23: **Colombia** (COP, `America/Bogota`, Ley 1581). Datos iniciales: Valle del Cauca (Cali, Yumbo) y Antioquia (Medellín, Itagüí); el administrador agrega más. |
| P6 | Reservas con < 24 h de antelación | Ver `booking-lifecycle.md` §6: mensaje de creación con botón de confirmar, sin recordatorio D-1; antelación mínima 60 min. |
| P7 | Estado tras reprogramar | ✅ Decidido 2026-11-24: cliente reprograma → mantiene `CONFIRMED` si lo estaba; negocio reprograma → `PENDING_CONFIRMATION`. |
| P8 | ¿Auto-cancelación de pendientes no confirmadas? | ✅ Decidido 2026-11-24: no en MVP. |
| P9 | ¿Opción "cualquier profesional" al reservar? | ✅ Decidido 2026-11-24: no; el cliente elige barbero y es obligatorio. |
| P10 | ¿Negocio nuevo queda `ACTIVE` al registrarse o requiere revisión admin? | ✅ Actualizado 2026-11-24 (ADR-0018): **el dueño crea su negocio** (autoservicio) con demo de 7 días en la que ya recibe reservas; también el administrador puede crearlo (nace `PENDING_REVIEW`). Sin pago al vencer: no visible y datos eliminados a los 30 días. Pagos en la Fase 11B. Miembros por email de un usuario registrado. |
| P11 | Crear una ausencia/bloqueo sobre citas existentes | ✅ Implementado en T6.7 (409 `AGENDA_CONFLICTS` con la lista). Decidido 2026-11-24: rechazar con lista de citas en conflicto (también cambios de horario); no hay entidad de bloqueos, una ausencia lo es. Retirar o desactivar un barbero sí se permite: sus citas quedan por reasignar. |
| P12 | ¿El cliente necesita cuenta para reservar? | ✅ Decidido 2026-11-24: sí, con email verificado y teléfono obligatorio; el negocio puede crear clientes sin cuenta en citas manuales. |
| P13 | Proveedor de email para PROD | **QA usa Mailpit** (decisión 2026-11-19). Para PROD elegir proveedor con SMTP+TLS antes de la Fase 13 (p. ej. Amazon SES, Resend, Postmark, Brevo); requiere dominio con SPF/DKIM. |
| P14 | ¿Los portales Next.js llaman a la API directamente (cookie + CORS) o mediante BFF? | Directamente con cookie `__Host-` en subdominio común; reevaluar en Fase 8. |
| P15 | Dominio para QA/PROD | A definir antes de Fase 3. |
| P16 | ¿Quién asume el costo de WhatsApp? ¿Tope por negocio? | Plataforma, con tope mensual por negocio configurable. |
| P17 | ¿Meta Business Manager verificado disponible? | 2026-11-25: app "booking-platform" creada, número de prueba funcionando ("Hello World" recibido), token permanente de usuario del sistema cargado en QA. **Pendiente:** verificación del negocio (RUT / Cámara de Comercio) y número real. 2026-11-24: el usuario **no tenía** cuenta de Meta Business; se le explicaron los pasos. Iniciar el trámite antes de la Fase 7 (la verificación puede tardar). |
| P18 | PROD: PostgreSQL en Droplet o DO Managed PostgreSQL | Decidir en Fase 13; recomendación: Managed. |
| P19 | ¿Commits firmados obligatorios? | Recomendado (SSH signing), no bloqueante. |
| P20 | ~~¿`booking-platform.code-workspace` en la raíz?~~ | ✅ Creado en la raíz del workspace, sin versionar. |

## Nuevas (2026-11-19, estrategia de ramas ADR-0017)

| # | Pregunta | Propuesta por defecto |
|---|---|---|
| P23 | ~~Protección de ramas en repo privado~~ | ✅ Respondida 2026-11-19: **sin protección** por ahora (disciplina + CI). |
| P24 | ~~Crear `qa` desde `main`~~ | ✅ Hecho 2026-11-19. |
| P25 | ~~Renombrar `feat/t1.4-database`~~ | ✅ Hecho 2026-11-19 (`feature/t1.4-database`, ya fusionada y borrada). |
| P26 | ~~GitHub CLI~~ | ✅ `gh` 2.105.0 instalado y autenticado (lhbotero1985). |
| P27 | Calibración final de Argon2id en el Droplet QA (objetivo 100–250 ms) | ✅ Resuelto 2026-11-23 (T3.7): 64 MiB, **t=2**, p=2 → 222–262 ms en el Droplet de 1 vCPU (t=3 → 335–347 ms). `ARGON2_ITERATIONS=2` en el compose QA; los hashes existentes se ajustan al iniciar sesión. PROD se recalibra en su hardware (Fase 13). |
| P28 | Ataques distribuidos de contraseña (muchas IP contra una cuenta) | El backoff es por cuenta+IP para no permitir bloquear a la víctima. Propuesta Fase 12: límite global suave por cuenta con CAPTCHA o aviso por email, no bloqueo. |
| P31 | ~~Política de contraseñas frente a ASVS L2~~ | ✅ Respondida 2026-11-19: **mínimo 8** y lista pequeña (excepción ASVS documentada; T2.10). |
| P32 | MFA para PLATFORM_ADMIN | ✅ Decidido 2026-11-19: **Fase 12** (TOTP obligatorio para administradores antes de PROD). |
| P33 | Listado y cierre selectivo de sesiones (ASVS V3.3.4) | ✅ Decidido 2026-11-19: **Fase 12** (`GET /me/sessions`, `DELETE /me/sessions/{id}`). |
| P38 | Copia externa de los backups QA | Hoy solo en el mismo Droplet (sin costo). Si se pierde el Droplet se pierden datos de QA (sintéticos). Opciones: (a) aceptar en QA (recomendado); (b) descarga manual periódica (`scp`); (c) DO Spaces (US$ 5/mes) u otro almacenamiento, obligatorio para PROD. |
| P40 | Invitar a la app al cliente agendado a mano | ✅ **Decidido 2026-11-24 (usuario):** apenas el barbero lo inscribe manualmente, se le envía por WhatsApp el enlace para descargar la app (Fase 7). Propuesta original: en la Fase 7, al crear una cita manual, enviar por WhatsApp al celular del cliente la cita y un enlace para descargar la app; si se registra con ese celular queda enlazado a su ficha (ya funciona). Alternativa: invitación por correo si el barbero lo escribió (no verificado). Enlace configurable mientras la app no está en las tiendas: `GET /app` (`APP_DOWNLOAD_URL`), implementado en T7.3 con la plantilla `cita_agendada_negocio`. |
| P42 | Palabra "barbero" en los correos y plantillas | ✅ **Decidido 2026-11-25 (usuario): "especialista".** Aplicado en los correos (T7.6). Pendiente: la plantilla de WhatsApp `cita_cambio_barbero` vive en Meta; cambiarla exige crear una nueva (p. ej. `cita_cambio_especialista`) y que Meta la apruebe; se hará junto con las demás plantillas. Los textos futuros (frontend, T7.7) usan "especialista". |
| P41 | Autorización de tratamiento de datos (Ley 1581 de 2012, habeas data) para WhatsApp | El usuario decidió (2026-11-24) que agendar es la aceptación. Recomendación: antes de PROD, revisar con un asesor legal el aviso de privacidad y el texto que la app y el barbero muestran al agendar ("recibirás mensajes de tu cita por WhatsApp"). No bloquea QA. |
| P39 | Proveedor de pagos, planes y precios | Fase 11B: Wompi, Mercado Pago o PayU (Colombia); precio mensual por negocio vs por profesional; facturación electrónica DIAN (validar con el contador). Requiere autorización de costos. |
| P37 | Protección de las ramas `qa`/`main` | ✅ Decidido 2026-11-23: **seguir así** (regla de proceso: solo merges de PR). El repo es privado en el plan Free: GitHub no aplica protección de ramas ni rulesets (verificado 2026-11-23: `qa` sin protección). Hoy la regla es de proceso (solo merges de PR). Como cada push a `qa` despliega, opciones: (a) seguir así; (b) GitHub Pro (US$ 4/mes) para protección y aprobadores; (c) repo público (no recomendado). |
| P34 | Tamaño del Droplet QA | ✅ Decidido 2026-11-22: **seguir con 1 GB + 2 GB de swap y validar**; subir a 2 GB (`s-1vcpu-2gb-intel`, US$ 14/mes) solo si las mediciones lo exigen. Medición T3.2: 650 MB usados, 28 MB de swap. |
| P35 | DNS gratuito para QA | ✅ Decidido 2026-11-22: **DuckDNS** `booking-platform-col.duckdns.org` → `174.138.92.121` (verificado). |
| P36 | Cloud Firewall de DigitalOcean | ✅ Decidido 2026-11-22: **solo firewall propio (`ufw`)**; Caddy será el único servicio expuesto (80/443). |
| P30 | Retención y acceso a `audit_log` | Propuesta Fase 12: 12 meses en línea, consulta solo PLATFORM_ADMIN (auditada), purga programada con River; revisar requisitos legales de protección de datos (Colombia, Ley 1581). |
| P29 | Límites por IP detrás de NAT (p. ej. wifi de un salón) | 10/min en endpoints de credenciales es holgado para pocos dispositivos; revisar con datos reales de QA. Requiere IP real vía Caddy (Fase 3). |
