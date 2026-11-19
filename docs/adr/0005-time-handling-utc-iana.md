# ADR-0005: Manejo de tiempo: UTC + zona IANA

- Estado: Aceptado (aprobación Fase 0, 2026-11-18) · Fecha: 2026-11-18

## Decisión
- Instantes en `timestamptz`, manipulados en UTC.
- Cada negocio tiene una zona IANA; horarios semanales en hora local (`time`) + día.
- Las ventanas se convierten a instantes con `time.Date(..., loc)`; los slots se generan con paso
  en tiempo absoluto desde el inicio de cada rango laboral.
- `Clock` inyectado; nunca `time.Now()` en el dominio.
- Base de datos tz embebida (`time/tzdata`) para resultados independientes del SO.
- Duraciones en minutos enteros, sin redondeo al intervalo.

## Consecuencias
- (+) Correcto ante DST y negocios en distintas zonas.
- (−) Casos borde DST requieren pruebas explícitas (ver `availability-engine.md` §4).
- Rangos laborales que cruzan medianoche no se soportan en el MVP.
