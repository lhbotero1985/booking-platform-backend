# Progreso

Registro cronológico. Una entrada por tarea. No borrar entradas anteriores.

---

## 2026-11-18 · Fase 0 · T0.1 — Diagnóstico del repositorio

- **Estado:** ✅ Completada
- **Hallazgos:**
  - `booking-platform/` contiene solo `booking-platform-backend/` y `booking-platform-frontend/`,
    ambas vacías. No existen `infrastructure/`, `docs/` raíz ni `booking-platform.code-workspace`.
  - No había repositorio Git.
  - Herramientas: Git 2.30.1, Docker 27.2.0, Docker Compose v2.29.2. **Go no está instalado.**
    golangci-lint, govulncheck, sqlc, goose, task, pre-commit: no verificados / no instalados.
  - No hay trabajo previo que preservar.
- **Archivos modificados:** ninguno.
- **Pruebas:** no aplica.

## 2026-11-18 · Fase 0 · T0.2 — Git y base DevSecOps

- **Estado:** ✅ Completada
- **Archivos:** `.gitignore`, `.gitattributes`, `.editorconfig`, `.gitleaks.toml`,
  `.pre-commit-config.yaml`, `.github/workflows/secret-scan.yml`, `.github/dependabot.yml`,
  `.github/pull_request_template.md`, `SECURITY.md`.
- **Acciones:** `git init -b main` en `booking-platform-backend/`; commit inicial local
  (sin remoto, sin push).
- **Pruebas:** gitleaks **no se pudo ejecutar** (Docker Desktop apagado); revisión manual por grep
  sin hallazgos. Ver `docs/testing/test-results.md`.
- **Problemas:** Git local 2.30.1 es antiguo (2021); se recomienda actualizarlo.

## 2026-11-18 · Fase 0 · T0.3 — Documentación de arquitectura

- **Estado:** ✅ Completada (pendiente de aprobación)
- **Archivos:** `CLAUDE.md`, `README.md`, `docs/architecture/*` (11 documentos),
  `docs/adr/*` (15 ADR + índice), `docs/plan/*`, `docs/testing/*`, `docs/operations/*`.
- **Pruebas:** no aplica (sin código).
- **Próximos pasos:** T0.4 — revisión del usuario y respuesta a preguntas pendientes.

## 2026-11-18 · Fase 0 · T0.4 — Respuestas del usuario (parcial)

- **Estado:** 🟡 En curso (falta aprobación explícita de la Fase 0)
- **Respuestas:** P1 monorepo · P2 privado · P3 instalación autorizada.
- **Acciones:**
  - `.git` movido de `booking-platform-backend/` a `booking-platform/` (historial conservado).
  - Archivos transversales movidos a la raíz: `.github/`, `.gitignore` (ampliado para Node/Expo/
    Terraform), `.gitattributes`, `.editorconfig`, `.gitleaks.toml`, `.pre-commit-config.yaml`,
    `SECURITY.md`. Nuevos: `README.md` raíz, `booking-platform.code-workspace`.
  - ADR-0014 reescrito y **Aceptado** (monorepo).
  - Herramientas instaladas: Go 1.27.4 (winget), golangci-lint 2.15.0 (winget), Task 3.54.0
    (winget), govulncheck v1.10.0, goose v3.30.1, sqlc v1.31.1 (`go install`).
- **No instalado:** `pre-commit` (requiere Python; pendiente de autorización). Git sigue en 2.30.1.
- **Problemas:** primer `go install` de govulncheck falló por DNS (`lookup proxy.golang.org`);
  el reintento funcionó. Nota: abrir una terminal nueva para que el PATH incluya Go.

## 2026-11-18 · Fase 0 · T0.4 — Aprobación

- **Estado:** ✅ Fase 0 aprobada por el usuario.
- **Aclaración del usuario:** workspace local único, pero **dos repos en GitHub** (backend y
  frontend). Se devolvió `.git` y la configuración DevSecOps a `booking-platform-backend/`
  (historial conservado). ADR-0016 reemplaza a ADR-0014; ADR 0001–0015 aceptados.
- `README.md` y `booking-platform.code-workspace` quedan en la raíz del workspace sin versionar.

## 2026-11-18 · Fase 1 · T1.1 — Toolchain y esqueleto del proyecto

- **Estado:** ✅ Completada
- **Archivos:** `go.mod`, `Taskfile.yml`, `.golangci.yml`, `cmd/api/main.go`,
  `cmd/worker/main.go`, `internal/platform/buildinfo/{buildinfo.go,buildinfo_test.go}`,
  `.github/dependabot.yml` (gomod), `CLAUDE.md` (comandos).
- **Decisiones:** módulo `booking-platform-backend` (provisional); golangci-lint v2 con gosec,
  depguard (pureza del dominio), forbidigo (`fmt.Print` → slog) y goimports con prefijo local;
  solo se crean carpetas con contenido.
- **Pruebas:** `task check` completo en verde; regla depguard verificada con violación
  intencional (archivo temporal eliminado). Ver `test-results.md`.
- **Problemas:** `go test -race` no es ejecutable localmente (sin gcc/cgo en Windows); se
  ejecutará en CI (T1.8). gitleaks sigue pendiente de Docker Desktop.
- **Próximos pasos:** T1.2.
