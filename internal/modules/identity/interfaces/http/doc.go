// Package httpapi exposes the Identity & Access use cases over REST (/api/v1/auth/*, /me):
// request DTOs, input validation, mapping of application errors to RFC 9457 problems and route
// registration. Handlers hold no business logic and never serialize domain entities directly.
package httpapi
