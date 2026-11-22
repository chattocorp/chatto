---
type: performance
---

Session and access-token validation now sends far fewer requests to NATS. After a replica fully checks a user, it checks that user's later requests against its own copy of the account state for 60 seconds. **Security change:** a password change, password reset, account deletion, deactivation, key shredding, or external identity unlink now revokes an existing session within 60 seconds in the worst case. Before, every replica rejected the session at its next request. The delay applies only on a replica that has not yet applied the change, which usually takes milliseconds. Sign-in, logout, session renewal, token refresh, and the periodic check of realtime connections still check the complete state each time. Two new metrics, `chatto_auth_generation_checks_total` and `chatto_auth_generation_strict_checks_total`, count the checks. No action is necessary.
