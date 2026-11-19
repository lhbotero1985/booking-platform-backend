# ADR-0001: Registrar decisiones con ADR

- Estado: Propuesto · Fecha: 2026-11-18

## Contexto
El proyecto se desarrollará por fases a lo largo de semanas, con sesiones de Claude Code que
deben retomar contexto sin pérdida.

## Decisión
Registrar cada decisión arquitectónica en `docs/adr/NNNN-titulo.md`. Las decisiones menores se
registran en `docs/plan/decisions.md`. Un ADR aceptado no se edita: se reemplaza con uno nuevo.

## Consecuencias
Trazabilidad de por qué existe cada pieza; costo bajo de mantenimiento.
