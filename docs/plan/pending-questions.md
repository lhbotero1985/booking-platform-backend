# Preguntas pendientes

Cada pregunta tiene una **propuesta por defecto**. Si el usuario no indica otra cosa al aprobar
la Fase 0, se adopta la propuesta.

## Bloqueantes para Fase 1

| # | Pregunta | Propuesta por defecto |
|---|---|---|
| P1 | ¿Repositorio solo backend (polyrepo) o monorepo en `booking-platform/`? (ADR-0014) | Monorepo si se quiere compartir OpenAPI sin publicar artefactos; polyrepo si se prefieren permisos/CI separados. **Recomendación: monorepo** — se puede migrar este historial sin perder commits. Requiere autorización para trabajar en la raíz. |
| P2 | ¿Repositorio GitHub privado o público? ¿Cuenta personal u organización? | Privado. (CodeQL y secret scanning avanzado en privados requieren GitHub Advanced Security; gitleaks cubre secretos en ambos casos.) |
| P3 | ¿Instalo Go y las herramientas en esta máquina (winget/scoop) o lo haces tú? | Lo instalas tú o autorizas `winget install GoLang.Go`; el resto vía `go install`. |
| P4 | ¿Actualizar Git local (2.30.1, de 2021)? | Sí, recomendado. |

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
| P13 | Proveedor de email (recuperación de contraseña) | Proveedor transaccional con capa gratuita, decidir en Fase 2 (p. ej. Resend, Amazon SES, Postmark). |
| P14 | ¿Los portales Next.js llaman a la API directamente (cookie + CORS) o mediante BFF? | Directamente con cookie `__Host-` en subdominio común; reevaluar en Fase 8. |
| P15 | Dominio para QA/PROD | A definir antes de Fase 3. |
| P16 | ¿Quién asume el costo de WhatsApp? ¿Tope por negocio? | Plataforma, con tope mensual por negocio configurable. |
| P17 | ¿Meta Business Manager verificado disponible? | Iniciar trámite antes de Fase 7. |
| P18 | PROD: PostgreSQL en Droplet o DO Managed PostgreSQL | Decidir en Fase 13; recomendación: Managed. |
| P19 | ¿Commits firmados obligatorios? | Recomendado (SSH signing), no bloqueante. |
| P20 | ¿Archivo `booking-platform.code-workspace` en la raíz? | Crearlo cuando se autorice tocar la raíz (ligado a P1). |
