---
type: fix
---

`WaitFor` no longer reads the target message from the stream leader when `LastSeq` already covers the position. A wait that has not ended after 100 ms reads the message once, in parallel with the wait, and still fails for a position that the projector cannot reach. A subject filter that cannot match a consumed subject fails at once without a read. A wrong subject and sequence pair is no longer reported when the projector is already past the position.
