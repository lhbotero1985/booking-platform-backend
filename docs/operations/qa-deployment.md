# Despliegue QA

Estado: **En construcción (Fase 3).** Servidor preparado en T3.1: ver `server-bootstrap.md`.

Contenido previsto:
1. Prerrequisitos (cuenta DO, dominio, llaves SSH, secretos del Environment `qa`).
2. Aprovisionamiento del Droplet: `deploy/server/bootstrap.sh` (`server-bootstrap.md`) ✅.
3. Endurecimiento (ver `docs/architecture/deployment.md` §3).
4. Primer despliegue y verificación.
5. Despliegues posteriores vía `deploy-qa.yml`.
6. Registro de despliegues (fecha, commit, digest, actor, resultado de smoke tests).
