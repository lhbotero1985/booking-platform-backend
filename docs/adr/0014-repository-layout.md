# ADR-0014: Monorepo para toda la plataforma

- Estado: **Reemplazado por ADR-0016** (2026-11-18) · Fecha: 2026-11-18

## Contexto
El workspace `booking-platform/` contiene backend, frontends, infraestructura y docs. Los cuatro
frontends consumen el mismo contrato OpenAPI del backend.

## Decisión
Un único repositorio Git **privado** en `booking-platform/` (GitHub: `booking-platform`).

- Archivos transversales en la raíz: `.github/`, `.gitignore`, `.gitattributes`,
  `.editorconfig`, `.gitleaks.toml`, `.pre-commit-config.yaml`, `SECURITY.md`, `README.md`,
  `booking-platform.code-workspace`.
- Cada proyecto conserva su documentación propia (`booking-platform-backend/docs`, `CLAUDE.md`).
- Los workflows de CI usan filtros `paths:` por proyecto (`booking-platform-backend/**`,
  `booking-platform-frontend/**`, `infrastructure/**`); los de seguridad (gitleaks) cubren todo
  el repositorio.
- El frontend consume `booking-platform-backend/api/openapi.yaml` directamente desde el repo.

## Alternativa descartada
Repositorios separados por proyecto: requería publicar el contrato OpenAPI como artefacto
versionado y coordinar cambios entre varios PR.

## Consecuencias
- (+) Un PR puede cambiar API y cliente a la vez; un solo lugar de CI, secretos y protecciones.
- (−) CI con filtros por ruta; permisos no granulares por proyecto (aceptable con un solo
  propietario).
- La regla 10 (no modificar fuera del backend) sigue vigente para el **contenido** de frontend
  e infraestructura; los archivos transversales de la raíz se modifican como parte del
  mantenimiento del repositorio.
- Migración realizada: el `.git` creado en el backend se movió a la raíz conservando el commit
  inicial (los archivos aparecen como renombrados).
