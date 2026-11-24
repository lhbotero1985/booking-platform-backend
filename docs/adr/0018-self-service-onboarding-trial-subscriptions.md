# ADR-0018: Alta autoservicio, demo de 7 días y suscripciones

- Estado: Aceptado (decisión del usuario, 2026-11-24) · Fecha: 2026-11-24
- Modifica P10 (antes: solo el administrador crea negocios) y la regla "pagos fuera del MVP"
  (riesgo R8). Complementa ADR-0007 (autorización) y ADR-0008 (jobs).

## Contexto
El modelo de negocio definido por el usuario: el dueño de una barbería entra al portal, crea su
negocio y lo configura durante **una semana de demo**; después debe pagar para seguir usándolo.
Decisiones del usuario (2026-11-24):
1. Durante la demo el negocio **puede recibir reservas reales** (es visible para clientes).
2. Si no paga al terminar la demo, los datos se conservan **30 días** y luego se eliminan.
3. El alta autoservicio entra en la Fase 4 (T4.3b) y los pagos en una fase propia antes de PROD.
4. `PLATFORM_ADMIN` puede hacer todo sobre todos los negocios.

## Decisión

### Dos ejes de estado, separados
| Eje | Módulo | Estados | Para qué |
|---|---|---|---|
| Moderación | `businesses` (`businesses.status`) | `PENDING_REVIEW`, `ACTIVE`, `SUSPENDED` | Decisión del administrador (calidad, abuso, soporte) |
| Suscripción | **nuevo módulo `subscriptions`** | `TRIALING`, `ACTIVE`, `PAST_DUE`, `EXPIRED`, `CANCELED` | Derecho a usar el servicio (demo, pago) |

Un negocio es **visible y reservable** solo si `businesses.status = ACTIVE` **y** su suscripción
está en `TRIALING`, `ACTIVE` o `PAST_DUE` (gracia mientras se reintenta un cobro). Separarlos evita
que un vencimiento de pago y una suspensión por abuso se pisen.

### Alta
- **Autoservicio (T4.3b):** cuenta verificada → `POST /api/v1/businesses` → negocio `ACTIVE` +
  membresía OWNER + suscripción `TRIALING` con `trial_ends_at = ahora + 7 días`, todo en una
  transacción (`db.ContextWithTx`). Límite: **una demo por cuenta** (si ya es OWNER de un negocio,
  debe pedir otro al soporte). Límite de peticiones estricto y auditoría.
- **Administrador (T4.3, opción de soporte):** puede crear un negocio a nombre de un dueño; nace
  igual que el autoservicio, `ACTIVE` con demo `TRIALING` (decisión del usuario 2026-11-24: el
  administrador no tiene que activar nada). El administrador puede suspender/reactivar, extender
  la demo o marcarla `ACTIVE` (cortesía) hasta que existan pagos.
- Para no mostrar barberías vacías, la búsqueda pública (T4.6/Fase 5) exige al menos un servicio
  activo.

### Vencimiento y retención
- Job periódico de River (cada hora): `TRIALING` con `trial_ends_at` vencido → `EXPIRED`
  (auditado). El aviso previo al dueño (2 días antes) llega con los canales de notificación
  (Fase 7) o con la fase de pagos (11B), lo que ocurra primero.
- `EXPIRED` sin pago durante **30 días** → eliminación de los datos del negocio (job; se
  implementa con la fase de pagos, cuando existan todas las tablas del tenant).

### Pagos (fase nueva "11B — Suscripciones y pagos", antes de las pruebas integrales y PROD)
- Proveedor por decidir (Wompi, Mercado Pago, PayU…), cobro recurrente, webhooks firmados e
  idempotentes, reactivación al pagar, `PAST_DUE` con reintentos.
- **Nunca** se manejan datos de tarjeta (checkout/tokenización del proveedor, PCI DSS SAQ-A).
- Facturación electrónica DIAN: validar obligación y proveedor con el contador del usuario.
- Requiere autorización de costos (cuenta del proveedor, comisiones).

### `PLATFORM_ADMIN`
Reemplaza la decisión del 2026-11-19 ("sin rol implícito en negocios"): `PLATFORM_ADMIN` puede
ejecutar **cualquier** caso de uso de cualquier negocio. Cada acción queda auditada con el
administrador como actor. Se implementa en el autorizador (T4.4).

## Consecuencias
- Nuevo bounded context `subscriptions` (tabla `subscriptions`, contract para consultar si un
  negocio está habilitado). Reservas (Fase 6) y búsqueda (T4.6) consultan ese contract.
- El riesgo R8 (crecimiento del alcance) se acepta explícitamente para pagos.
- Riesgos nuevos: abuso de demos (mitigado con email verificado, una demo por cuenta, límites);
  dependencia de un proveedor de pagos; obligaciones fiscales.
