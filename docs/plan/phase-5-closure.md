# Cierre de la Fase 5 — Profesionales y servicios

Fecha: 2026-11-24 · Estado: **✅ Aprobada por el usuario (2026-11-24)**

## 1. Entregables

### Flujo principal: el responsable deja su barbería lista para recibir reservas

1. Crea su barbería (Fase 4). Al nacer **no aparece en la búsqueda pública**: todavía no hay
   nada que reservar.
2. Agrega a sus barberos como miembros (cada barbero tiene su propia cuenta) y les crea el
   perfil profesional: nombre visible, descripción, activo/inactivo y orden. Si él también
   atiende, se registra como profesional.
3. Define el horario semanal y los descansos de cada barbero, en la zona horaria del negocio.
4. Registra ausencias e incapacidades (fecha y hora de inicio y fin, y motivo).
5. Arma su catálogo: categorías opcionales y servicios con duración y precio en pesos.
   "Corte + barba" es un servicio más, con su propio precio y duración.
6. Indica qué servicios presta cada barbero. Desde ese momento la barbería **aparece en la
   búsqueda pública**, con un catálogo público de servicios, precios y barberos.

El barbero, con su propia cuenta, ve la configuración y puede ajustar **su** horario, **sus**
descansos y **sus** ausencias. No puede tocar las del responsable ni las de otro barbero (403).

Validado de punta a punta en QA el 2026-11-24 con el script E2E: una cuenta nueva y su barbería,
recorriendo todos los pasos anteriores, **135/135**.

### Tareas

| Tarea | Entregable | Estado | Evidencia |
|---|---|---|---|
| T5.1 | Se elimina el rol MANAGER: solo OWNER (responsable) y PROFESSIONAL (barbero) | ✅ | PR #40, E2E 95/95 |
| T5.2 | Profesionales: perfil ligado a la cuenta de un miembro; solo OWNER gestiona; auditoría | ✅ | PR #41, E2E 101/101 |
| T5.3 | Horario semanal y descansos; OWNER para todos, el barbero el suyo; sin solapes; descansos dentro de una franja; escrituras serializadas (`FOR UPDATE`) | ✅ | PR #42, E2E 110/110 |
| T5.4 | Ausencias e incapacidades; sin solapes garantizado por la base de datos (exclusion constraint) | ✅ | PR #43, E2E 118/118 |
| T5.5 | Catálogo: categorías opcionales, servicios con nombre único, duración de 5 a 480 min en múltiplos de 5 y precio en pesos enteros | ✅ | PR #44, E2E 126/126 |
| T5.6 | Servicios que presta cada barbero (solo OWNER asigna; mismo negocio garantizado por FK compuestas; reemplazos concurrentes serializados) | ✅ | PR #45, E2E 129/130 (falla del script, no de la API) |
| T5.7 | Catálogo público y búsqueda que exige algo reservable | ✅ | PR #46, E2E 135/135 |
| T5.8 | Suite de aislamiento ampliada (IDOR dentro del propio negocio), verificación completa, este documento | ✅ | `feature/t5.8-phase5-closure` |

Módulo nuevo: `catalog`. Contratos nuevos entre módulos: `professionals.Directory` y
`professionals.Bookable`, `businesses.Currencies` y `businesses.PublicDirectory`, y
`catalog.Offerings`. Migraciones 00010–00015. API: **60 rutas, todas documentadas en
`api/openapi.yaml`** (eran 39 al cerrar la Fase 4).

## 2. Pruebas

| Tipo | Estado | Evidencia (`docs/testing/test-results.md`) |
|---|---|---|
| Unitarias | ✅ | Cobertura: professionals domain 99,2 %, application 94,1 %; catalog domain 100 %, application 93,9 %; businesses domain 95,1 %, application 90,4 %; identity application 83,6 % |
| Integración (PostgreSQL real) | ✅ | Esquema (CHECK, FK compuestas, unicidad sin distinguir mayúsculas, exclusion constraint); **concurrencia** en horarios, ausencias y asignaciones, **verificada por mutación** (sin el bloqueo o la restricción, la prueba falla); búsqueda y catálogo público con todos los módulos reales |
| Contrato OpenAPI | ✅ | Todas las operaciones nuevas validadas con respuestas reales; rutas ↔ contrato idénticas en ambas direcciones (60) |
| Seguridad (`tests/security`) | ✅ | Aislamiento entre negocios en las **26** operaciones `{businessId}`; **nuevo en T5.8:** IDOR dentro del propio negocio en las **16** operaciones con ids de hijos (barbero, ausencia, categoría, servicio o miembro de otro negocio → 404 idéntico a inexistente) y referencias ajenas en el cuerpo (servicio o categoría de otro negocio rechazados); catálogo público sin campos internos |
| E2E en QA (HTTPS público) | ✅ | **135/135** tras desplegar T5.7 (`acceb44`); se repite tras desplegar T5.8 |
| Estático y dependencias | ✅ | golangci-lint (depguard por módulo), sqlc vet/diff, govulncheck, gitleaks, Trivy, shellcheck |

Nota: los handlers HTTP no tienen pruebas en su propio paquete (0 % allí); se ejercitan con las
pruebas de contrato, de seguridad y el E2E.

## 3. Checklist de seguridad de la fase

| Control | Verificación |
|---|---|
| Recurso de otro negocio → 404 idéntico a inexistente; sin cambios; denegación auditada | `TestEveryBusinessRouteIsTenantIsolated` (todas las rutas `{businessId}` del contrato, automático para rutas nuevas) |
| Ids de otro negocio usados dentro del propio (IDOR) → 404 idéntico a inexistente | Misma prueba, bloque nuevo de T5.8 (verificado por mutación: si se omite el filtro por negocio, falla con 200) |
| Integridad entre negocios garantizada también por el esquema | FK compuestas `(business_id, id)` en horarios, ausencias, servicios, categorías y asignaciones |
| Solo OWNER gestiona profesionales, catálogo y asignaciones; el barbero solo su propia agenda (403 en la ajena) | Pruebas de aplicación y E2E |
| Sin doble registro bajo concurrencia | `FOR UPDATE` (horario), exclusion constraint (ausencias), índices únicos (nombres), advisory lock (asignaciones); verificados por mutación |
| Vista pública sin datos internos (sin cuentas, emails, estados ni fechas) | `TestPublicSearchExposesOnlyBookableBusinesses`, E2E |
| Auditoría en la misma transacción que cada cambio | `professional.*`, `catalog.*` en pruebas de aplicación |

## 4. Decisiones del usuario incorporadas

- Roles: administrador de la plataforma, responsable (OWNER, puede ser también barbero), barbero
  (PROFESSIONAL) y cliente. Todo barbero tiene cuenta propia. Se eliminó MANAGER.
- El responsable gestiona todo; el barbero ajusta su horario, descansos y ausencias, y agregará
  citas manualmente (Fase 6).
- Ausencia: fecha y hora de inicio y fin, y motivo.
- Cada servicio tiene una sola duración y un solo precio para todos los barberos. Los combos son
  servicios normales.
- Catálogo: categorías opcionales, nombre único, duración de 5 a 480 min en múltiplos de 5 y
  precio en pesos enteros.
- El responsable asigna qué servicios presta cada barbero.
- La búsqueda pública exige al menos un servicio activo prestado por un barbero reservable.

## 5. Riesgos aceptados y deuda

| Tema | Estado |
|---|---|
| Ausencia registrada sobre citas existentes | Fase 6 (P11: rechazar y listar las citas en conflicto) |
| Citas manuales del barbero | Fase 6 |
| Aviso "tu demo vence en 2 días" | Fase 7 (notificaciones) u 11B |
| Eliminación de datos 30 días después de vencer la demo | Fase 11B |
| Un negocio sin oferta no aparece en la búsqueda, pero su ficha por slug sigue abierta | Aceptado (quien tiene el enlace lo ve) |
| Datos de prueba en QA: cada corrida E2E deja negocios, barberos y servicios sintéticos | Aceptado en QA; limpieza opcional |
| P37 (sin protección de ramas), P38 (backups en el mismo disco) | Aceptados en QA |

## 6. Aprobación

- [x] Usuario aprueba el cierre de la Fase 5 (2026-11-24); pide desglosar la Fase 6.
