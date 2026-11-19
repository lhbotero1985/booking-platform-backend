# ADR-0015: Identificadores UUIDv7 y dinero en unidades menores

- Estado: Propuesto · Fecha: 2026-11-18

## Decisión
- IDs `uuid` versión 7 generados en la aplicación (`google/uuid`): no enumerables, ordenables
  por tiempo, buen comportamiento en índices B-tree.
- Dinero como `bigint` en unidades menores + moneda ISO 4217; cada negocio opera en una moneda.

## Consecuencias
- (+) Sin errores de punto flotante; IDs seguros para exponer en URLs.
- (−) Conversión de presentación en los frontends (helper compartido).
