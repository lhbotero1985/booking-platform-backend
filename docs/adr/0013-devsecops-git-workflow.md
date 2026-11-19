# ADR-0013: DevSecOps y flujo Git

- Estado: Aceptado (aprobación Fase 0, 2026-11-18) · Fecha: 2026-11-18
- Nota (2026-11-19): el punto "Trunk-based con `main` protegida" fue reemplazado por ADR-0017.

## Decisión
- Trunk-based con `main` protegida, PR obligatorio, squash merge, Conventional Commits.
- Controles automáticos: gitleaks (pre-commit y CI), golangci-lint con gosec, govulncheck,
  Dependabot, Trivy (config e imagen), SBOM; CodeQL si el plan de GitHub lo permite.
- Acciones de GitHub con permisos mínimos y fijadas por SHA; GitHub Environments con aprobación
  para QA y PROD; secretos solo en Environments y servidores.
- PROD despliega exactamente la imagen (digest) validada en QA.

## Consecuencias
- (+) Seguridad verificada en cada cambio, trazabilidad completa.
- (−) PRs algo más lentos; aceptable.

Detalle: `docs/architecture/devsecops.md`.
