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
| P5 | País(es) y moneda(s) del piloto | Un país inicial; define zona horaria, moneda, ley de datos y plantillas de WhatsApp. |
| P6 | Reservas con < 24 h de antelación | Ver `booking-lifecycle.md` §6: mensaje de creación con botón de confirmar, sin recordatorio D-1; antelación mínima 60 min. |
| P7 | Estado tras reprogramar | Cliente reprograma → mantiene `CONFIRMED` si lo estaba; negocio reprograma → `PENDING_CONFIRMATION`. |
| P8 | ¿Auto-cancelación de pendientes no confirmadas? | No en MVP. |
| P9 | ¿Opción "cualquier profesional" al reservar? | Fuera del MVP (cada reserva elige profesional, como dice el prompt). |
| P10 | ¿Negocio nuevo queda `ACTIVE` al registrarse o requiere revisión admin? | `PENDING_REVIEW` → activación por admin (control de calidad del piloto). |
| P11 | Crear una ausencia/bloqueo sobre citas existentes | Rechazar con lista de citas en conflicto; el negocio debe reprogramarlas/cancelarlas primero. |
| P12 | ¿El cliente necesita cuenta para reservar? | Sí desde la app; el negocio puede crear clientes sin cuenta en reservas manuales. |
| P13 | Proveedor de email para PROD | **QA usa Mailpit** (decisión 2026-11-19). Para PROD elegir proveedor con SMTP+TLS antes de la Fase 13 (p. ej. Amazon SES, Resend, Postmark, Brevo); requiere dominio con SPF/DKIM. |
| P14 | ¿Los portales Next.js llaman a la API directamente (cookie + CORS) o mediante BFF? | Directamente con cookie `__Host-` en subdominio común; reevaluar en Fase 8. |
| P15 | Dominio para QA/PROD | A definir antes de Fase 3. |
| P16 | ¿Quién asume el costo de WhatsApp? ¿Tope por negocio? | Plataforma, con tope mensual por negocio configurable. |
| P17 | ¿Meta Business Manager verificado disponible? | Iniciar trámite antes de Fase 7. |
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
| P27 | Calibración final de Argon2id en el Droplet QA (objetivo 100–250 ms) | Medir en Fase 3 y fijar `ARGON2_ITERATIONS`; los hashes existentes se actualizan solos al iniciar sesión. |
| P28 | Ataques distribuidos de contraseña (muchas IP contra una cuenta) | El backoff es por cuenta+IP para no permitir bloquear a la víctima. Propuesta Fase 12: límite global suave por cuenta con CAPTCHA o aviso por email, no bloqueo. |
| P30 | Retención y acceso a `audit_log` | Propuesta Fase 12: 12 meses en línea, consulta solo PLATFORM_ADMIN (auditada), purga programada con River; revisar requisitos legales de protección de datos (Colombia, Ley 1581). |
| P29 | Límites por IP detrás de NAT (p. ej. wifi de un salón) | 10/min en endpoints de credenciales es holgado para pocos dispositivos; revisar con datos reales de QA. Requiere IP real vía Caddy (Fase 3). |
