---
type: feature
---

Members can choose which protected roles to activate in Shield Mode for up to 15 minutes. The event log shows which roles they selected. Role managers can protect individual roles. The `owner` role always requires activation, `everyone` grants apply during ordinary use, and role denies stay in force.

After upgrading, review custom roles and direct account grants. Enable **Require Shield Mode** on roles that need it. To require activation for a direct grant, including a management permission, move it to a protected role.

Bot managers can grant any permission the bot’s owner holds at the same scope, including permissions from inactive protected roles. If the owner loses a permission, the bot’s saved grant becomes inactive.
