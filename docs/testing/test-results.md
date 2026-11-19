# Resultados de pruebas

Solo resultados **reales** de comandos ejecutados. Formato: fecha · fase/tarea · comando ·
resultado · observaciones.

---

## 2026-11-18 · Fase 0 · T0.2

| Comando | Resultado | Observaciones |
|---|---|---|
| `docker run … gitleaks:v8.24.0 dir /repo` | ⚠️ **No ejecutado** | Docker Desktop no estaba en ejecución (`dockerDesktopLinuxEngine` no encontrado). Repetir con Docker activo o al primer push (workflow `security`). |
| `grep -rniE "(password\|secret\|token)\s*[:=]\s*['\"][^'\"]{8,}"` | ✅ Sin coincidencias | Revisión manual sustituta, no equivale a gitleaks. |

No existen pruebas de código: la Fase 0 no contiene código funcional.
