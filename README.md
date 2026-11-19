# booking-platform

Monorepo de una plataforma SaaS multiempresa de reservas para barberías, salones de belleza y
negocios de servicios con cita.

> Estado: **Fase 0 — Arquitectura.** Solo documentación de diseño; sin código funcional.

| Carpeta | Contenido | Estado |
|---|---|---|
| [booking-platform-backend/](booking-platform-backend/) | API y worker en Go | Diseño (Fase 0) |
| `booking-platform-frontend/` | Apps Expo y portales Next.js | A partir de la Fase 8 |
| `infrastructure/` | Configuración por ambiente (dev, qa, prod) | A partir de la Fase 3 |
| `docs/` | Documentación transversal | Cuando haga falta |

- Punto de entrada técnico: [booking-platform-backend/CLAUDE.md](booking-platform-backend/CLAUDE.md)
- Arquitectura: [booking-platform-backend/docs/architecture/overview.md](booking-platform-backend/docs/architecture/overview.md)
- Seguridad: [SECURITY.md](SECURITY.md)
