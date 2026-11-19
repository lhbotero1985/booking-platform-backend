# Backup y restauración

Estado: **Diseño — se implementa en la Fase 3 (QA) y 13 (PROD).**

## Diseño

| Aspecto | QA | PROD |
|---|---|---|
| Método | `pg_dump -Fc` diario | `pg_dump -Fc` diario + (❓) WAL/PITR o Managed PG |
| Cifrado | `age` con llave pública en servidor; privada fuera del servidor | Igual, llave distinta |
| Destino | DO Spaces (bucket/prefijo QA, credenciales QA) | DO Spaces (bucket PROD, credenciales PROD) |
| Retención | 7 días | 30 días diarios + 12 mensuales |
| Prueba de restauración | En cada cambio del procedimiento | Mensual, en ambiente aislado |
| Backup pre-migración | Siempre | Siempre |

Objetivos iniciales PROD: RPO ≤ 24 h (≤ 5 min si se adopta PITR), RTO ≤ 2 h.

## Procedimiento de restauración

Se documentará paso a paso y se ejecutará realmente en QA durante la Fase 3; el resultado se
registra en `docs/testing/test-results.md`.
