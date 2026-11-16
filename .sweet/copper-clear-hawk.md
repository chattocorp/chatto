---
type: feature
---

Botto can post GitHub push summaries as short root messages with a source link and a detailed thread reply in a configured Chatto room. The webhook verifies signatures, filters the repository and branch, saves queued work, and prevents replayed deliveries from creating duplicate posts. Uncertain sends require operator review. Configure the dedicated listener and webhook secret to enable announcements.

Conversation agents now have read-only access to Chatto MCP. The host continues to deliver thread replies and progress messages; agents cannot use MCP to post extra messages or perform other Chatto writes.

Botto can start and supervise an optional Cloudflare Tunnel connector with the webhook listener. The connector restarts after failure and stops with Botto, using a scoped token from local configuration.

Announcement sessions support operator-owned prompt files and selected MCP connections without owner-only tools. Klaus uses its GitHub read tools to inspect commit patches and relevant Sweet Release notes. Channel updates start with **✅ Just merged:** and identify the affected projects in a short benefit summary. The thread explains observable behavior, practical impact, and required upgrade actions. It omits internal implementation inventories and discloses unavailable diff inspection.

Channel summaries are limited to 400 characters before the source link. Root and reply deliveries have separate durable receipts, so a restart resumes a pending reply without repeating the channel post.

Botto can also announce published GitHub releases and prereleases in the configured room. Enable `releases` and set `release_prompt_file` in the existing announcement configuration. Klaus posts a short project and version summary with an inline release link, then highlights and useful upgrade guidance in a thread. Duplicate deliveries and release edits do not produce new posts; promotion from prerelease to stable can produce one new announcement.

Klaus checks GitHub changes before starting an announcement agent and skips merge and push announcements unless the push adds or modifies a Sweet Release note. The required path pattern is configurable. Failed or incomplete GitHub reads stay queued for retry. Botto saves skipped decisions without posting a root message or reply, including across restart and redelivery. Published release announcements are unchanged.
