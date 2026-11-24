# Siguiente tarea

## Última tarea completada
T5.4 — Ausencias e incapacidades (2026-11-24), en `feature/t5.4-absences` (commit y push; PR y
merge pendientes de autorización).

**Fase 5 en curso** (desglose aprobado 2026-11-24, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T5.5 — Catálogo de servicios

- Categorías opcionales (nombre único por negocio, orden, activa).
- Servicios: nombre (único por negocio), descripción, duración en minutos y precio en COP
  (enteros en unidades menores + `currency` del negocio), activo/inactivo, orden.
- "Corte + barba" es un servicio normal con su propio precio y duración (decisión 2026-11-24);
  la duración la da el servicio y es igual para todos los barberos.
- Solo el OWNER (o PLATFORM_ADMIN) gestiona; el equipo lo consulta; otro negocio 404; auditoría.
