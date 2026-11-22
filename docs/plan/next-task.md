# Siguiente tarea

## Última tarea completada
T3.2 — Stack QA en Docker Compose (2026-11-22), en `feature/t3.2-qa-compose-stack` (commit y
push hechos; PR y merge pendientes de autorización). Stack desplegado en el servidor QA.

**Fase 3 en curso** (desglose aprobado 2026-11-22, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t3.2-qa-compose-stack` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t3.1-qa-server-bootstrap` (local y remota).

## Próxima tarea: T3.3 — Caddy, HTTPS e IP real del cliente

- Servicio `caddy` (imagen fijada por digest) en el stack QA: único servicio que publica
  80/443; certificado Let's Encrypt para `booking-platform-col.duckdns.org` (HTTP-01);
  redirección HTTP→HTTPS; HSTS; compresión; límites de cuerpo y timeouts; logs JSON sin
  cabeceras sensibles; volumen para certificados.
- API: confiar en `X-Forwarded-For` **solo** desde el proxy (config `TRUSTED_PROXIES` con la
  red de Docker); el resto de clientes sigue usando el par TCP. Pruebas de suplantación.
- Quitar la publicación `127.0.0.1:8080` de la API cuando Caddy sea el punto de entrada.
- Pruebas: certificado válido, redirección, HSTS, cabeceras de seguridad, rate limiting y
  auditoría con la IP real, puertos.
