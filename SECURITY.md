# Política de seguridad

## Reporte de vulnerabilidades

No abrir issues públicos para vulnerabilidades. Reportarlas de forma privada mediante
**GitHub Security Advisories** ("Report a vulnerability") del repositorio, o al contacto que el
propietario del repositorio designe.

Incluir: descripción, pasos de reproducción, impacto estimado y versión/commit afectado.

## Compromisos

- Respuesta inicial en un máximo de 5 días hábiles.
- Corrección priorizada según severidad (CVSS).
- Los secretos expuestos se rotan inmediatamente, aunque el commit se haya revertido.

## Prácticas aplicadas (DevSecOps)

Ver [docs/architecture/devsecops.md](docs/architecture/devsecops.md): escaneo de secretos
(gitleaks), SAST (gosec vía golangci-lint), análisis de dependencias (govulncheck, Dependabot),
escaneo de imágenes (Trivy), SBOM, principio de mínimo privilegio en CI y ramas protegidas.
