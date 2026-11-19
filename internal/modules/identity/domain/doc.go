// Package domain holds the Identity & Access domain model: User, Session, PasswordResetToken,
// Membership and their value objects (Email, password policy, roles) and invariants
// (docs/architecture/domain-model.md §2).
//
// Rules (enforced by depguard): pure Go only — no database, HTTP, River or platform imports;
// no time.Now() (a Clock is injected through the application layer).
package domain
