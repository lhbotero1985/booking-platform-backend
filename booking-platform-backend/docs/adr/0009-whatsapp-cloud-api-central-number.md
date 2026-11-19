# ADR-0009: WhatsApp Cloud API con número central

- Estado: Propuesto · Fecha: 2026-11-18

## Decisión
Meta WhatsApp Business Platform vía Cloud API, con un número central de la plataforma. Cada
mensaje identifica al negocio en el contenido de la plantilla. Botones quick-reply con tokens
opacos de un solo uso. Webhooks verificados con HMAC-SHA256.

## Alternativas
- Número por negocio (Embedded Signup / Tech Provider): mejor marca, mucha más complejidad;
  evaluar después del piloto.
- BSP intermediario: costo adicional por mensaje.

## Consecuencias
- (+) Integración única, menor costo.
- (−) Requiere verificación de empresa en Meta, plantillas aprobadas y gestión de calidad del
  número; los clientes verán un remitente de la plataforma.
- Costos por mensaje: control con topes y kill switch.
