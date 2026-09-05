# Instructions for Agents Working in `proto/chatto/admin/v1/`

Follow [the shared protocol rules](../../../AGENTS.md) for API shape, comments,
compatibility, and code generation. This package is a public administrative
API; its namespace does not make the routes private.

- Keep administrative services in `chatto.admin.v1`. Do not move ordinary
  integration APIs here only because the bundled frontend uses them.
- Name services for the administrative resource. A service that manages user
  identity, passwords, roles, and membership must have a name that covers users.
- `chatto.api.v1.MyAccountService` owns current-user self-service operations.
  Administrative user management belongs in an admin service. State required
  permissions in RPC comments.
- Reuse `chatto.api.v1` messages when their meaning and visibility match.
  Add admin-specific messages only for admin-only fields or different
  authorization rules. Explain the difference in the message comment.
