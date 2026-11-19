// Package application holds the Identity & Access use cases (register, login, logout,
// password reset, membership management), their ports (repositories, password hasher, clock,
// mailer, audit) and the authorization policies of each use case.
//
// Rules (enforced by depguard): may import domain and other modules' contract packages only;
// no HTTP, database drivers or River.
package application
