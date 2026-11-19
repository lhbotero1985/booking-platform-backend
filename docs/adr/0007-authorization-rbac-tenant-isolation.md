# ADR-0007: Autorización RBAC y aislamiento multiempresa

- Estado: Aceptado (aprobación Fase 0, 2026-11-18) · Fecha: 2026-11-18

## Decisión
- Roles: `PLATFORM_ADMIN` (global), `OWNER`, `MANAGER`, `PROFESSIONAL` (por membresía en
  negocio), `CUSTOMER` (implícito).
- Políticas explícitas por caso de uso en la capa de aplicación, funciones puras probadas.
- Todo acceso a datos de negocio va acotado por `business_id` en SQL. Recurso de otro negocio
  → 404.
- Membresías consultadas por request (sin claims cacheados).
- Row-Level Security de PostgreSQL **diferido**; se reevaluará en Fase 12 como defensa adicional.

## Consecuencias
- (+) Simple, explícito y testeable.
- (−) Depende de disciplina en consultas; mitigado con suites de seguridad cross-tenant
  obligatorias por módulo y FK compuestas `(business_id, id)`.
