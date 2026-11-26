---
type: upgrade
---

**Review custom roles and direct account grants after the upgrade.** Enable **Require Shield Mode** on each role that needs activation. Move any direct grant that should require activation into a protected role. A bot can receive permissions its owner holds at the same scope, including through an inactive protected role. If the owner loses a permission, the bot’s grant becomes inactive.
