---
type: feature
---

Configure `owners.dids` (`CHATTO_OWNERS_DIDS`) to grant the owner role to AT Protocol accounts by DID. Handles are not accepted, and removing a configured DID does not revoke the role. SSO-only servers with a configured owner email or DID show provider sign-in without the setup wizard. On new servers, the first configured owner sign-in shows the owner welcome and activates Shield Mode once. Existing servers and explicit `core.skip_setup_wizard` installations keep their current welcome behavior.
