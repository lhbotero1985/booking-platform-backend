# Cierre de la Fase 4 — Geografía y negocios

Fecha: 2026-11-24 · Estado: **Pendiente de aprobación del usuario**

## 1. Entregables

### Flujo principal: el dueño crea su barbería (sin intervención del administrador)

1. Se registra y verifica su email.
2. Crea su barbería (`POST /api/v1/businesses`): queda **activa al instante**, él es su
   responsable (OWNER) y empieza la **demo de 7 días**.
3. Los clientes la encuentran en la búsqueda pública mientras la demo esté vigente.
4. Configura su negocio, agrega responsables, gerentes y barberos.
5. A los 7 días, sin pago, la demo vence sola (job horario) y deja de ser visible.

Validado en vivo en QA el 2026-11-24 con una cuenta nueva y sin usar la cuenta administradora.

### Tareas

| Tarea | Entregable | Estado | Evidencia |
|---|---|---|---|
| T4.1 | Geografía: países, departamentos y ciudades con zona horaria; datos iniciales Colombia (Valle del Cauca: Cali, Yumbo; Antioquia: Medellín, Itagüí); lectura pública, escritura `PLATFORM_ADMIN` auditada; comando `api admin grant-platform-admin` | ✅ | PR #32 |
| T4.2 | Agregado de negocio (slug, teléfono E.164, coordenadas, zona horaria, moneda, políticas); ubicación coherente garantizada por FK compuestas; regla `IN_USE` | ✅ | PR #33 |
| T4.3b | **Alta autoservicio** con demo de 7 días (ADR-0018): módulo `subscriptions`, una demo por cuenta, vencimiento horario (River) | ✅ | PR #35 |
| T4.3 | Opción de soporte: el administrador crea un negocio a nombre de un dueño (también nace `ACTIVE` con demo); suspender/reactivar (moderación); transacción compartida entre módulos (`db.ContextWithTx`) | ✅ | PR #34, ajuste en T4.7 |
| T4.4 | Gestión del negocio por su personal; `PLATFORM_ADMIN` puede actuar sobre cualquier negocio | ✅ | PR #36 |
| T4.5 | Miembros: varios responsables (OWNER), siempre al menos uno; gerentes y barberos; retiro inmediato | ✅ | PR #37 |
| T4.6 | Búsqueda pública: negocios activos con demo vigente o pagada, cursor, vista reducida | ✅ | PR #38 |
| T4.7 | Suite de aislamiento guiada por el contrato, verificación rutas ↔ OpenAPI, alta del administrador también `ACTIVE`, este documento | ✅ | `feature/t4.7-phase-4-closure` |

Módulos nuevos: `geography`, `businesses`, `subscriptions` (+ `internal/platform/audit`).
Migraciones 00005–00009. API: **39 rutas, todas documentadas en `api/openapi.yaml`**
(verificado automáticamente en ambas direcciones).

## 2. Pruebas

| Tipo | Estado | Evidencia (`docs/testing/test-results.md`) |
|---|---|---|
| Unitarias | ✅ | Cobertura: businesses domain 95,1 %, application 90,2 %; geography domain 100 %, application 91 %; subscriptions domain 100 %, application 89,2 %, worker 90,9 %; identity application 83,3 % |
| Integración (PostgreSQL real) | ✅ | Datos iniciales, FK de ubicación, `location` PostGIS, unicidades, **atomicidad** negocio + OWNER + demo + auditoría, **concurrencia** (una demo por cuenta; dos OWNER que se retiran a la vez), SQL del vencimiento, búsqueda con intercalación real |
| Contrato OpenAPI | ✅ | Todas las operaciones nuevas validadas con respuestas reales; rutas ↔ contrato idénticas |
| Seguridad (`tests/security`) | ✅ | Matriz de credenciales inválidas sobre todas las rutas autenticadas; solo admin; asignación masiva; **aislamiento entre negocios en todas las operaciones `{businessId}` del contrato** (404 idéntico a inexistente, sin cambios, denegación auditada) |
| E2E en QA (HTTPS público) | ✅ | **92/92** con la cuenta administradora (`f4db9ea`); flujo del dueño validado en vivo sin administrador; se repite tras desplegar T4.7 |
| Estático y dependencias | ✅ | golangci-lint (depguard por módulo), sqlc vet/diff, govulncheck, gitleaks, Trivy, shellcheck |

Nota de cobertura: los handlers HTTP de miembros (`identity/interfaces/http`, 58 % en su propio
paquete) se ejercitan con las pruebas de contrato y de seguridad de otros paquetes.

## 3. Checklist de seguridad de la fase

| Control | Verificación |
|---|---|
| Aislamiento multiempresa: recurso de otro negocio → 404 idéntico a inexistente (ADR-0007) | `TestEveryBusinessRouteIsTenantIsolated` (todas las rutas del contrato), pruebas por módulo |
| Autorización en el caso de uso, membresías leídas en cada petición; retiro y revocación inmediatos | `TestMembersAreTenantScoped`, `TestPlatformAdminRevocationIsImmediate` |
| `PLATFORM_ADMIN` solo por comando de servidor, auditado; puede actuar sobre cualquier negocio (ADR-0018) | `TestSetPlatformAdmin*`, `TestPlatformAdminActsOnAnyBusiness` |
| Campos controlados por el servidor no asignables (estado, slug, moneda, zona, dueño, suscripción…) | `TestBusinessCreationRejectsServerControlledFields`, `TestSelfServiceOnboardingRules`, `TestBusinessManagementIsTenantScoped` |
| Auditoría en la misma transacción que el cambio; rollback si falla | `TestWriteRollsBackWhenAuditFails`, `TestCreationIsAtomicWithTheDemo` |
| Integridad por esquema (FK compuestas, CHECK, unicidades) | Pruebas de integración de esquema |
| Búsqueda pública sin datos internos y sin inyección de patrones | `TestPublicSearchExposesOnlyBookableBusinesses` |
| Límite estricto en el alta autoservicio; una demo por cuenta con bloqueo | `TestSelfServiceOnboardingOnPostgres` (concurrencia) |

## 4. Decisiones del usuario incorporadas

- P5: piloto Colombia con Valle del Cauca y Antioquia.
- ADR-0018: alta autoservicio con demo de 7 días (con reservas reales), retención 30 días tras
  vencer, Fase 11B de pagos, `PLATFORM_ADMIN` con poder total sobre los negocios.
- Responsable del negocio = OWNER, varios permitidos, siempre al menos uno; cambio de dueño =
  nuevo responsable. MANAGER solo lectura de la configuración.
- Todo negocio nace activo con demo; el administrador no tiene que activar nada (revisión del
  cierre, 2026-11-24). Su papel: soporte y moderación (suspender/reactivar, extender demos).

## 5. Riesgos aceptados y deuda

| Tema | Estado |
|---|---|
| Aviso "tu demo vence en 2 días" | Fase 7 (notificaciones) u 11B, lo que ocurra primero |
| Eliminación de datos 30 días después de vencer | Fase 11B (cuando existan todas las tablas del tenant) |
| La búsqueda exige al menos un servicio activo | Fase 5 (catálogo) |
| Cambio de slug por el administrador | No implementado (sin necesidad actual) |
| Datos de prueba en QA: cada corrida E2E deja 1 negocio suspendido y 1 autoservicio (su demo vence sola) | Aceptado en QA; limpieza opcional |
| Revelar a un OWNER si un email tiene cuenta al agregar miembros | Aceptado por el usuario (P10) |
| P37 (sin protección de ramas), P38 (backups en el mismo disco) | Aceptados en QA |

## 6. Aprobación

- [ ] Usuario aprueba el cierre de la Fase 4.
