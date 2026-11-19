# ADR-0008: Jobs en segundo plano con River (outbox transaccional)

- Estado: Propuesto · Fecha: 2026-11-18

## Decisión
River sobre la misma base PostgreSQL. Los jobs se insertan con `InsertTx` en la misma
transacción del cambio de dominio (outbox nativo). Worker en binario separado `cmd/worker`.
Reintentos con backoff, unicidad por clave de idempotencia, jobs programados para
recordatorios, colas separadas por tipo.

## Alternativas
- Outbox propio + poller: reinventa River.
- Redis/Asynq, RabbitMQ, Kafka: infraestructura adicional sin necesidad.

## Consecuencias
- (+) Sin infraestructura extra; atomicidad entre cita y notificación.
- (−) Carga adicional en PostgreSQL (baja para el volumen del MVP).
