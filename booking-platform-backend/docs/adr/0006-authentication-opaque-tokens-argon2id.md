# ADR-0006: Autenticación con tokens opacos y Argon2id

- Estado: Propuesto · Fecha: 2026-11-18

## Decisión
- Contraseñas con Argon2id (formato PHC, parámetros calibrados ≥ OWASP).
- Sesiones en PostgreSQL; el cliente recibe un token opaco aleatorio de 256 bits; se guarda solo
  su SHA-256.
- Expiración por inactividad y absoluta, distintas para móvil, web y admin.
- Móvil: header Bearer. Web: cookie `__Host-` HttpOnly/Secure/SameSite + CSRF.

## Alternativas
- JWT: no revocables sin lista negra; riesgo de claims obsoletos.
- Proveedor externo (Auth0, Cognito, Firebase Auth): costo y dependencia; reconsiderable si se
  requiere login social.

## Consecuencias
- (+) Revocación inmediata, simple de razonar y auditar.
- (−) Una consulta por request para validar sesión (indexada; aceptable).
