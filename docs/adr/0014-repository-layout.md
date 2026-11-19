# ADR-0014: Repositorio Git independiente para el backend

- Estado: Propuesto · Fecha: 2026-11-18

## Contexto
El workspace `booking-platform/` contiene backend, frontends, infraestructura y docs. La regla
10 del prompt restringe el trabajo a `booking-platform-backend/` en las primeras fases.

## Decisión (provisional)
Inicializar Git en `booking-platform-backend/` como repositorio propio
(`booking-platform-backend` en GitHub). Frontends e infraestructura tendrán sus propios
repositorios cuando se autoricen. El contrato OpenAPI se publica como artefacto versionado
(release del backend) que los frontends consumen.

## Alternativa
Monorepo en `booking-platform/`. Ventajas: contrato OpenAPI compartido sin publicación, un solo
PR para cambios transversales, un solo lugar de CI. Desventajas: CI con filtros por ruta,
permisos menos granulares.

## Consecuencias
Pregunta abierta para el usuario (ver `docs/plan/pending-questions.md`). Si se elige monorepo,
el historial de este repositorio puede moverse a un subdirectorio conservando los commits
(`git subtree` / `git filter-repo`).
