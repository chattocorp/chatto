---
type: feature
---

Botto can post GitHub push summaries as short root messages and a detailed thread reply with a source link in a configured Chatto room. The webhook verifies signatures, filters the repository and branch, saves queued work, and prevents replayed deliveries from creating duplicate posts. Uncertain sends require operator review. Configure the dedicated listener and webhook secret to enable announcements.

Conversation agents now have read-only access to Chatto MCP. The host continues to deliver thread replies and progress messages; agents cannot use MCP to post extra messages or perform other Chatto writes.

Botto can start and supervise an optional Cloudflare Tunnel connector with the webhook listener. The connector restarts after failure and stops with Botto, using a scoped token from local configuration.

Announcement sessions support operator-owned prompt files and selected MCP connections without owner-only tools. Botto supplies the changed Sweet Release notes at the exact new commit to Klaus. Klaus uses their paths and contents to summarize project scope, benefits, and upgrade guidance, with GitHub read tools available for clarification. Channel updates start with **✅ Just merged:** and identify the affected projects in a short benefit summary. The thread explains observable behavior, practical impact, and required upgrade actions. It omits internal implementation inventories.

Channel summaries are limited to 400 characters. Root and reply deliveries have separate durable receipts, so a restart resumes a pending reply without repeating the channel post.

Botto can also announce published GitHub releases and prereleases in the configured room. Enable `releases` and set `release_prompt_file` in the existing announcement configuration. Klaus posts a short project and version summary with an inline release link, then highlights and useful upgrade guidance in a thread. Duplicate deliveries and release edits do not produce new posts; promotion from prerelease to stable can produce one new announcement.

Klaus checks GitHub changes before starting an announcement agent and skips channel announcements unless the push adds or modifies a Sweet Release note. The required path pattern is configurable. Failed or incomplete GitHub reads stay queued for retry. Botto saves skipped decisions without posting a root message or reply, including across restart and redelivery. Published release announcements are unchanged.

Botto follows human replies in threads it opens for announcements, without requiring another mention. Participation survives restarts, and existing confirmed announcement threads are registered on startup. Klaus treats reports of missed messages as uncertain behavior to investigate instead of inventing routing explanations.

Enable `thread_replies` to let Klaus notify the original reporting thread when an owner-authorized issue has a verified fix on the configured branch. Issue origins and delivery receipts use the existing Means collection. Later pushes can catch up on missed notifications, including pushes without new release notes. A fix notification does not imply that the change is released or deployed.

Issue-thread notifications reject message bodies that contain only destination metadata before sending. Rejected metadata-only messages do not consume the delivery key. Notification instructions also require complete final text and prohibit placeholder or test sends.
