---
type: feature
---

Botto can post GitHub push summaries as short root messages with a source link and a detailed thread reply in a configured Chatto room. The webhook verifies signatures, filters the repository and branch, saves queued work, and prevents replayed deliveries from creating duplicate posts. Uncertain sends require operator review. Configure the dedicated listener and webhook secret to enable announcements.

Conversation agents now have read-only access to Chatto MCP. The host continues to deliver thread replies and progress messages; agents cannot use MCP to post extra messages or perform other Chatto writes.

Botto can start and supervise an optional Cloudflare Tunnel connector with the webhook listener. The connector restarts after failure and stops with Botto, using a scoped token from local configuration.

Announcement sessions support operator-owned prompt files and selected MCP connections without owner-only tools. Klaus uses its GitHub read tools to inspect commit patches and writes updates with a consistent Markdown format, including a disclosure when diff inspection is unavailable.

Channel summaries are limited to 400 characters before the source link. Root and reply deliveries have separate durable receipts, so a restart resumes a pending reply without repeating the channel post.
