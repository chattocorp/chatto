---
type: performance
---

Authenticated requests send far fewer requests to NATS: after a replica fully checks a user, it checks that user's next requests for 60 seconds against its own copy of the account state. **Security change:** a password change, password reset, account deletion, deactivation, or external identity unlink now revokes existing sessions and tokens within 60 seconds in the worst case. Before, every replica rejected them at their next request. The delay applies only on a replica that has not yet applied the change, which usually takes milliseconds. No action is necessary.
