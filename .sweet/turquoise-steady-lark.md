---
type: breaking
---

The bundled frontend now hides the Server Gutter and the Server Directory by default and shows only the server that serves it. To let users add and switch between other Chatto servers again, set `frontend.gutter.enabled = true` or `CHATTO_FRONTEND_GUTTER_ENABLED=true`. The standalone frontend and the desktop app are not affected.
