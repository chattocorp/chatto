---
type: fix
---

Outgoing SMTP email now uses the domain of the configured sender address in its `Message-ID` header instead of the machine hostname. In containers, the hostname is usually a container ID, which can cause receivers such as Gmail to reject the message. You no longer need to set a container `hostname` to work around this.
