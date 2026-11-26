<!-- Copia sincronizada de booking-platform-frontend/docs/BACKEND_CONTRACT_PROPOSALS.md (2026-11-26). Solo documentación; el frontend la actualiza por PR cada vez que cambia. -->

# Propuestas de contrato para el backend

**Documento vivo: aquí el frontend registra todo lo que necesita ajustar del backend** (decisión del
usuario, 2026-11-26). Se mantiene una copia idéntica en el backend
(`booking-platform-backend/docs/plan/frontend-contract-proposals.md`), actualizada por PR de solo
documentación cada vez que cambia; el backend decide cuándo implementarlo y el usuario fusiona esos
PR. Nada de esto está implementado mientras no aparezca en el roadmap del backend.

Cada propuesta sigue las convenciones del contrato (OpenAPI 3.1, `additionalProperties: false`,
RFC 9457, paginación por cursor, 404 entre negocios, auditoría de acciones admin).

Última actualización: 2026-11-26 (P-6; fases del frontend renumeradas: la web va primero, F5–F10,
y las apps después, F11–F12).

Orden sugerido por impacto en el frontend: **P-6 (G-23, G-24; bloquea F6, en curso)** → **P-1
(G-13)** → **P-2 (G-10)** → **P-5 (G-11)** → **P-4 (G-15)** → **P-3 (G-05)**.

| ID | Brecha | Tipo de cambio | Compatible con v1 | Bloquea |
|---|---|---|---|---|
| P-1 | G-13 Membresías sin datos del negocio | Campos nuevos opcionales en respuesta existente | ✅ | F6 (selector de negocio), F12 |
| P-2 | G-10 Admin no ve geografía inactiva | Endpoints nuevos de lectura | ✅ | F10 |
| P-3 | G-05 Sin `PATCH /me` | Endpoint nuevo | ✅ | F11, F12 (perfil) |
| P-4 | G-15 Sin detalle de cita para el personal | Endpoint nuevo | ✅ | F8, F12 |
| P-5 | G-11 Sin usuarios ni auditoría para el admin | Endpoints nuevos | ✅ | F10 |
| P-6 | G-23 Correos de identidad sin plantilla; G-24 enlace al dominio de la API en QA | Presentación de correos y configuración (sin cambio de API) | ✅ | F6 (requisito del usuario) |

---

## P-1 — Datos del negocio en `GET /api/v1/me/memberships` (G-13)

**Problema.** `Membership` solo trae `businessId` y `role`. Para mostrar el selector de negocio
(portal y Booka Pro) el frontend debe hacer una llamada `GET /business/{id}` por membresía (N+1) y
no sabe si el negocio está suspendido o con la demo vencida.

**Propuesta.** Agregar un objeto `business` resumido (campos de solo lectura ya públicos para el
miembro):

```yaml
Membership:
  type: object
  additionalProperties: false
  required: [businessId, role, business]
  properties:
    businessId: { type: string, format: uuid }
    role: { type: string, enum: [OWNER, PROFESSIONAL] }
    business:
      type: object
      additionalProperties: false
      required: [name, slug, status, timeZone, entitled]
      properties:
        name: { type: string }
        slug: { type: string }
        status: { $ref: "#/components/schemas/BusinessStatus" }
        timeZone: { type: string }
        entitled:
          type: boolean
          description: Subscription allows the business to operate now (demo running or paid).
```

**Notas.** Agregar `business` como **requerido** es un cambio aditivo para clientes que ignoran
campos (el frontend actual no existe aún). Consulta en lote a `businesses` y `subscriptions` vía
sus `contract` (como `EntitledAmong` en la búsqueda) para evitar N+1 en el servidor.

**Pruebas sugeridas.** Contrato; miembro retirado no aparece; negocio suspendido aparece con
`status: SUSPENDED`.

---

## P-2 — Lecturas administrativas de geografía con inactivos (G-10)

**Problema.** Las lecturas públicas devuelven solo registros activos. Un `PLATFORM_ADMIN` que
desactiva un país, división o ciudad ya no puede verlo para reactivarlo.

**Propuesta.** Tres lecturas bajo `/api/v1/admin` con los mismos esquemas (`Country`, `Division`,
`City` ya incluyen `active`):

| Método | Ruta | Respuesta |
|---|---|---|
| GET | `/api/v1/admin/countries` | `CountryList` (activos e inactivos) |
| GET | `/api/v1/admin/countries/{code}/divisions` | `DivisionList` |
| GET | `/api/v1/admin/divisions/{id}/cities` | `CityList` |

Seguridad `bearerAuth` / `sessionCookie`; 401, 403 `FORBIDDEN` si no es `PLATFORM_ADMIN`, 404 si
el padre no existe. Las listas son pequeñas (sin paginación, como las públicas). Opcional:
`?active=true|false`.

**Pruebas sugeridas.** No admin → 403; inactivo visible solo aquí; agregar las rutas a la suite
de contrato (rutas = operaciones).

---

## P-3 — `PATCH /api/v1/me` (G-05)

**Problema.** El usuario no puede corregir su nombre (se muestra al negocio en la agenda) ni elegir
idioma.

**Propuesta.**

```yaml
/api/v1/me:
  patch:
    operationId: updateMe
    summary: Update the current account
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            additionalProperties: false
            minProperties: 1
            properties:
              fullName: { type: string, minLength: 1, maxLength: 120 }
              locale: { type: string, enum: [es-CO], description: "Grows with i18n." }
    responses:
      "200": { content: { application/json: { schema: { $ref: "#/components/schemas/MeResponse" } } } }
      "400": { $ref: "#/components/responses/BadRequest" }
      "401": { $ref: "#/components/responses/Unauthorized" }
```

Email y teléfono **no** cambian aquí (el teléfono ya tiene `PUT /me/phone`; el email requeriría
reverificación). Auditoría `account.updated` sin valores personales.

**Duda para el backend.** ¿El nombre del `Customer` del negocio (copiado al reservar) debe seguir
al de la cuenta? Propuesta: no; el negocio conserva su ficha.

---

## P-4 — Detalle de una cita para el personal (G-15)

**Problema.** El portal y Booka Pro solo obtienen una cita desde la agenda (por rango). Un enlace
directo (notificación push, URL compartida en el portal) no puede cargar la cita.

**Propuesta.**

| Método | Ruta | Roles | Respuesta |
|---|---|---|---|
| GET | `/api/v1/business/{businessId}/appointments/{appointmentId}` | OWNER (cualquiera), PROFESSIONAL (solo su agenda; ajena → 403 como en las transiciones) | `AgendaItem` + `version` y datos de cancelación (o el esquema `Appointment` existente) |
| GET | `/api/v1/business/{businessId}/customers/{customerId}/appointments?scope=upcoming\|past&cursor=&limit=` | OWNER, PROFESSIONAL | Lista paginada de `AgendaItem` |

Otro negocio → 404 idéntico a inexistente. Entra automáticamente en la suite de aislamiento
basada en el contrato (operaciones con `{businessId}`).

---

## P-5 — Usuarios y auditoría para el administrador (G-11)

**Problema.** El portal admin no puede buscar cuentas (soporte) ni consultar `audit_log`.
Relacionado con P30 (retención y acceso a auditoría, Fase 12 del backend).

**Propuesta (mínima).**

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/api/v1/admin/users?q=&status=&cursor=&limit=` | Búsqueda por email exacto o prefijo de nombre; devuelve `id`, `email`, `fullName`, `status`, `isPlatformAdmin`, `createdAt`, `memberships[]` (businessId, role). Sin teléfono completo (enmascarado) |
| POST | `/api/v1/admin/users/{id}/suspend` · `/reactivate` | Suspender revoca sesiones (ya existe la regla en dominio); idempotente; auditado |
| GET | `/api/v1/admin/audit-log?businessId=&actorId=&action=&from=&to=&cursor=&limit=` | Solo `PLATFORM_ADMIN`; **la consulta misma se audita** (P30); sin datos sensibles (ya no se guardan) |

**Riesgo.** Mientras no exista MFA para administradores (P32), estas capacidades amplían el
impacto de una cuenta admin comprometida; se recomienda implementarlas junto con MFA o limitar el
portal admin a QA hasta entonces (ver R-09 en `KNOWN_ISSUES.md`).

---

## P-6 — Correos de identidad con la plantilla de marca y enlaces al portal (G-23, G-24)

**Problema (verificado en Mailpit de QA, 2026-11-26).** «Confirma tu correo electrónico»,
«Restablece tu contraseña» y «Tu contraseña fue cambiada» salen en texto plano
(`identity/application/registration.go` y `password.go`), mientras los correos de citas ya usan la
plantilla HTML de Booka (`notifications/infrastructure/email/render.go`). Además, en QA el enlace
apunta a `https://booking-platform-col.duckdns.org/verify-email?token=…`, que responde 404.

**Requisito del usuario.** Estos correos deben venir con la plantilla de correos de la plataforma y
su enlace debe abrir las páginas del portal con el diseño de Booka.

**Propuesta.**
1. Reutilizar el layout de los correos de citas (logo, colores de marca, pie con contacto) para los
   tres correos de identidad, en HTML y texto alternativo (multipart), con:
   - Verificación: saludo con el nombre, botón «Confirmar mi correo», vigencia (24 h), «si no creaste
     esta cuenta, ignora este mensaje».
   - Recuperación: botón «Crear una nueva contraseña», vigencia del enlace, aviso de seguridad.
   - Contraseña cambiada: aviso con fecha y hora en la zona del usuario y qué hacer si no fue él.
   - El enlace completo también como texto (clientes de correo sin HTML) y nunca en el asunto.
2. QA: `PUBLIC_APP_URL=https://booka-app-qa.duckdns.org` en cuanto el frontend publique
   `/verify-email` y `/reset-password` (F6.2 y F6.4). Producción: el dominio de business-web.
3. Sin cambios de API ni de tokens: el frontend consume `token` de la URL tal como hoy.

**Criterio de aceptación.** En Mailpit de QA, los tres correos llegan en HTML con la marca y su
enlace abre `https://booka-app-qa.duckdns.org/verify-email` o `/reset-password`, que muestran la
página del portal.

## Siguientes brechas (no incluidas aún)

G-04 planes (llega con la Fase 11B; la landing usa por ahora precios estáticos decididos por el
usuario), G-02 dispositivos (T7.6), G-07 filtros de búsqueda, G-08 categoría o sector del negocio,
G-09 imágenes: logo, portada y foto de profesionales (implica almacenamiento con costo), G-12
indicadores, G-16 estado de configuración, G-18 reseñas, calificaciones y favoritos. Las
referencias de diseño del usuario (asistente de configuración, app de clientes) muestran G-08,
G-09 y G-18; se propondrán con contrato cuando el usuario las priorice.
