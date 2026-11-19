# ADR-0002: Monolito modular Go con DDD y Clean Architecture

- Estado: Aceptado (aprobación Fase 0, 2026-11-18) · Fecha: 2026-11-18

## Contexto
Equipo pequeño, dominio en descubrimiento, operación crítica (reservar) que exige transacciones
ACID. El prompt maestro excluye microservicios, Kubernetes y Kafka.

## Decisión
Un único módulo Go con siete bounded contexts en `internal/modules/*`, capas
`domain/application/infrastructure/interfaces` y un paquete `contract` como única superficie
pública entre módulos. Dos binarios: `cmd/api` y `cmd/worker`. Fronteras verificadas con
`depguard` en golangci-lint.

## Alternativas
- Microservicios: costo operativo y consistencia distribuida injustificados.
- Monolito por capas sin módulos: acoplamiento creciente, difícil de extraer luego.

## Consecuencias
- (+) Transacciones locales, despliegue simple, refactor barato.
- (+) Posible extracción futura de módulos (p. ej. Notifications).
- (−) Requiere disciplina en las fronteras; mitigado con lint y revisión.
