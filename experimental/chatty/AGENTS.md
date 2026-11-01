# Chatty development

Chatty is an experimental terminal client in its own Go module. Read
[README.md](README.md) for current behavior, commands, and limits.

- Keep reusable authentication, HTTP, pagination, and realtime behavior in
  `../chatto-go`. Keep terminal layout, input, and private local storage here.
- Preserve Gobotto's message filtering when extending SDK observation for humans.
- Never log credentials, profile data, message bodies, or authorization URLs.
  Authentication errors must not expose response bodies or callback codes.
- Keep S256 PKCE, state validation, loopback binding, and listener cancellation.
  Never use a shell to launch a browser or follow API redirects with credentials.
- Persist refresh request identity before rotation. Persist an outgoing payload
  and idempotency key before sending. Do not retry uncertain sends automatically.
- Keep session storage private and atomic. Never clear a pending send because
  another room was selected. Keep stale request responses out of the current view.
- Sanitize terminal control characters in server-provided text before rendering.
- Render room labels with no space between `#` and the room name: `#testing`.
  Apply this rule in the sidebar, pane titles, and all other room labels.
- Use `mise check` here and in `../chatto-go`; verify the build with CGO disabled.
  Keep tests synthetic. Do not log in or send messages as the user for UI tests.
