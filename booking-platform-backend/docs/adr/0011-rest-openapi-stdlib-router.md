# ADR-0011: REST spec-first OpenAPI 3.1 con router stdlib

- Estado: Propuesto · Fecha: 2026-11-18

## Decisión
- Contrato en `api/openapi.yaml` (OpenAPI 3.1), escrito antes que los handlers.
- Router `net/http` de la stdlib; middleware propio mínimo.
- Handlers escritos a mano; las pruebas de integración validan peticiones y respuestas contra el
  spec (validador con soporte 3.1, a seleccionar en Fase 1).
- Errores RFC 9457.

## Alternativas
- Generación de servidor (oapi-codegen): se reevaluará en Fase 1 según su soporte de 3.1.
- Frameworks (Gin, Echo, Chi): innecesarios con el router actual de la stdlib.

## Consecuencias
- (+) Contrato estable para cuatro frontends; menos dependencias.
- (−) Riesgo de deriva spec↔código; mitigado con validación en pruebas.
