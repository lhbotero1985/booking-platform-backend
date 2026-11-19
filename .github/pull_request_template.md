## Tarea

Fase / ID de tarea: <!-- p. ej. Fase 1 · T1.3 -->

## Qué cambia y por qué

## Pruebas ejecutadas (resultados reales)

- [ ] `task test`
- [ ] `task test:race`
- [ ] `task test:integration`
- [ ] `task lint` / `task vuln` / `task secrets`

## Checklist de seguridad

- [ ] Sin secretos, credenciales ni datos reales en el diff.
- [ ] Toda consulta de datos de negocio filtra por `business_id`.
- [ ] Autorización verificada en backend (pruebas de acceso no autorizado / cross-tenant).
- [ ] Entradas validadas; SQL solo parametrizado.
- [ ] Logs sin datos sensibles.

## Documentación

- [ ] `docs/plan/progress.md` y `docs/plan/next-task.md` actualizados.
- [ ] ADR / `decisions.md` si hubo decisiones.
- [ ] `docs/testing/test-results.md` con resultados reales.
- [ ] `api/openapi.yaml` actualizado si cambió la API.
