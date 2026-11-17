---
type: feature
---

Standalone frontend containers now publish a page title, description, and link-preview image. The defaults describe Chatto. Set CHATTO_FRONTEND_TITLE, CHATTO_FRONTEND_DESCRIPTION, or CHATTO_FRONTEND_IMAGE to change them; set CHATTO_FRONTEND_ORIGIN so that previews can use absolute URLs.

Standalone frontend containers can now run on a read-only root file system. Mount writable temporary file systems at /var/cache/nginx and /var/run.

Chatto servers now set the page title and description from the server name and description.
