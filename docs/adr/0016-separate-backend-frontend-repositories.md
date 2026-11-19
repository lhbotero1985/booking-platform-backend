# ADR-0016: Repositorios separados para backend y frontend

- Estado: Aceptado (decisión del usuario, 2026-11-18) · Fecha: 2026-11-18 · Reemplaza a ADR-0014

## Contexto
El usuario aclaró que el workspace local es uno solo (`booking-platform/`), pero en GitHub
existirán **dos repositorios privados**: uno para el backend y otro para el frontend.

## Decisión
- `booking-platform-backend/` es un repositorio Git independiente → GitHub
  `booking-platform-backend` (privado).
- `booking-platform-frontend/` será otro repositorio independiente → GitHub
  `booking-platform-frontend` (privado). Se inicializa en la Fase 8.
- La carpeta raíz `booking-platform/` es solo el workspace local; `README.md` y
  `booking-platform.code-workspace` de la raíz no se versionan.
- Infraestructura: se propone dentro del repositorio backend (`deploy/qa`, `deploy/prod`), ya
  que despliega el backend. Se confirma en la Fase 3.
- Contrato API: el frontend consume `api/openapi.yaml` del backend; cada release del backend
  publica el spec como artefacto versionado y el frontend genera su cliente desde una versión
  fijada.
- Cada repositorio tiene su propia configuración DevSecOps (`.github/`, gitleaks, Dependabot,
  `SECURITY.md`).

## Consecuencias
- (+) Permisos, CI y ciclos de release independientes por repositorio.
- (−) Los cambios de API que afectan al frontend requieren dos PR coordinados por versión
  del contrato.
- El `.git` que se había movido a la raíz volvió a `booking-platform-backend/` conservando el
  historial (los commits intermedios muestran los archivos renombrados).
