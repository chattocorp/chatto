// Package rbac provides a generic RBAC (Role-Based Access Control) engine.
//
// The engine provides:
//   - Role CRUD operations (create, get, list, update, delete)
//   - Permission grants/revokes per role
//   - Role assignments per user
//   - Optional user-level permission overrides (grant/deny)
//
// Designed as a generic two-tier-capable engine (instance + space) before
// Phase 5 of #330 collapsed RBAC into a single server tier. Today only one
// adapter remains — `core/rbac.go` wraps the engine for the unified
// SERVER_RBAC bucket. The multi-tier abstraction is over-engineered for
// the current single-consumer reality; a future cleanup could inline this
// engine into `core/rbac.go`.
package rbac
