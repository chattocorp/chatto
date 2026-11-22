# GitHub announcements

Botto can post a short root message to a configured Chatto room after a GitHub
push or published release, then add details as a thread reply.
The operator selects one repository, branch, and destination. This configuration
is permission to post announcements for matching pushes. Conversation participants
cannot select another destination or use Chatto MCP write tools.

## Configuration

Add this table to `botto.toml`:

```toml
[announcements.github]
enabled = true
repository = "chattocorp/chatto"
branch = "main"
room_id = "YOUR_ROOM_ID"
webhook_secret = "${BOTTO_GITHUB_WEBHOOK_SECRET}"
host = "127.0.0.1"
port = 5152
```

Set `BOTTO_GITHUB_WEBHOOK_SECRET` in the local `.env` file to a random secret
of at least 32 characters. This is a separate credential from the GitHub App
private key. The bot account must have permission to post in the destination.
Restart Botto after configuration changes.

Route a public HTTPS endpoint to `POST /github` on the configured host and port.
Use a TLS reverse proxy or a tunnel. Do not expose the Runling console for this
purpose. Configure a GitHub repository webhook with that URL, JSON content type,
the same secret, and the **push** event. For releases, also subscribe to the
**release** event in the webhook or GitHub App settings. Keep SSL verification enabled.
The listener supports GitHub's signed ping request.

The listener verifies the SHA-256 signature over the original request bytes.
It accepts bodies up to 1 MiB. Only matching repository and branch pushes enter
the queue. Branch deletions and unchanged heads do not produce announcements.
A successful response is `202`; the queue is saved before this response.
Retry failed webhook deliveries through GitHub after the listener is available.

## Release announcements

Release announcements are opt-in. Add these fields to the existing
`[announcements.github]` table:

```toml
releases = true
release_prompt_file = "prompts/release-announcement.md"
```

Releases use the same endpoint, secret, repository, room, and selected MCP
connections as push announcements. No new port or tunnel route is needed.
The listener accepts `published` for stable releases and prereleases, and
`released` for stable releases. Drafts and other actions, including edits,
deletions, and unpublishing, do not produce posts. The push branch filter still
applies to pushes; releases are selected by repository, not by their
`target_commitish` field.

The signed release ID and prerelease status identify each announcement.
Repeated deliveries and the published/released pair create only one post.
Promotion from prerelease to stable can create one new stable announcement.
An edit or a repeated publication at the same stage does not create a new post.

The agent receives the tag, title, publication time, prerelease status, and up
to 20,000 characters of release notes, with a truncation flag. Author and asset
records are omitted. Release notes and tool results go to the configured model
provider and are subject to the same privacy constraints as push evidence.

Klaus's [release prompt](prompts/release-announcement.md) uses the release notes
as its main source. Stable messages start with **🚀 Released:**; prerelease
messages start with **🧪 Prerelease:**. The root names the project, version,
and main benefit. The host adds a **Release notes** link in the same paragraph.
The reply gives short **Highlights** and **Upgrade notes** only when useful.
It focuses on user and operator effects, not internal implementation details.
A release does not imply that it is deployed, the latest version, or ready for
production use.

## Content and delivery

A separate model session writes the announcement. Botto passes the repository,
branch, immutable before/after commit IDs, and commit messages to it. Botto does
not pass full diffs to the model. When a path filter is configured, it also
supplies the matching file contents. The operator's prompt and selected MCP tools guide the agent's research and writing.

For Klaus, add these fields to `[announcements.github]` in `botto.toml`:

```toml
prompt_file = "prompts/commit-announcement.md"
mcp_servers = ["github"]
```

The prompt path is relative to `botto.toml`. Restart Botto after editing the prompt.
The named connections must exist under `[mcp.servers]`. Only their ordinary
`tools` are exposed; `admin_tools` are excluded. Configure those ordinary grants
for read access. Announcement sessions do not receive the built-in Chatto or Exa
connections, conversation history, skills, or ambient local tools. If no MCP
connections are selected, the session has no research tools.

Klaus's [announcement prompt](prompts/commit-announcement.md) uses the changed
release notes supplied by the host as its main source. It calls GitHub read tools
only to resolve a specific ambiguity or missing detail. The agent finishes with
the `finish_announcement` tool, which supplies a channel summary and separate
thread details. The host requires a Markdown summary of at most 400 characters
and details of at most 4,000 characters. Intermediate model text is not posted.
For pushes, the host appends a comparison link (or a commit link for a new branch)
at the end of the thread details, in a separate paragraph. The root summary has
no source link. Release announcements keep their release-notes link in the root
summary.

Klaus's local configuration requires a newly added Markdown release note with
frontmatter `type: highlight` under a release unit's `.sweet/` directory. This check runs before the model
session starts. To use the same filter, add these fields to
`[announcements.github]`:

```toml
required_changed_path = '(^|/)\.sweet/[^/]+\.md$'
required_changed_content = "^---\\r?\\n(?:(?!---\\r?(?:\\n|$))[^\\n]*\\n)*?type:[ \\t]*(?:highlight|\"highlight\"|\\x27highlight\\x27)[ \\t]*(?:#[^\\r\\n]*)?\\r?\\n"
added_files_only = true
github_app_id = "${BOTTO_GITHUB_APP_ID}"
github_installation_id = "${BOTTO_GITHUB_APP_INSTALLATION_ID}"
github_private_key_path = "${BOTTO_GITHUB_APP_PRIVATE_KEY_PATH}"
```

The path and content settings are regular expressions; Botto does not hardcode
Sweet Release. The content expression above matches `type: highlight` only in
the first frontmatter block. `added_files_only` excludes edits to existing notes.
The key path is relative to the TOML file. The GitHub App must be installed on
the configured repository with Contents read permission. These settings can use
the same environment variables as the GitHub MCP connection. Botto creates a
short-lived installation token scoped to that repository with Contents read
permission. No additional credential is needed for Klaus.

Botto requests the GitHub comparison for the signed before/after SHAs. If the
changed-file list reaches GitHub's limit, or the push diverges from its old head,
it compares the two Git trees instead. Added, modified, and renamed matching
files qualify by default; `added_files_only = true` selects only added paths.
Deleted files alone and unchanged notes do not qualify. API failures and
incomplete trees keep the event queued for retry without starting a model.
For a qualifying push, Botto fetches every matched file at the immutable `after`
SHA and supplies its path and UTF-8 contents in `changedFiles`. It accepts at most
100 files, 64 KiB per file, and 256 KiB in total. Missing, invalid, or oversized
contents keep the event queued; they do not start an agent with partial notes.
Only files that also match `required_changed_content`, when configured, enter
the model context. Both additional settings require `required_changed_path`.
A complete negative result saves a `skipped` receipt and posts neither channel
message. With `thread_replies = true`, the agent first checks issue follow-ups
and can notify original threads even when no release note qualifies.
Repeated deliveries and restarts preserve that decision. Existing prepared
messages resume delivery without a new eligibility check.
Published releases bypass this push filter.

The filter sends the repository and immutable commit IDs to GitHub. Its
responses remain in the Botto process. With thread follow-ups disabled, skipped
events do not start a model session. With follow-ups enabled, push metadata and
requested memory and GitHub tool results enter the model context even when the
channel announcement is skipped. Matched file contents go to the configured
model provider for qualifying events.
Klaus can use read tools for clarification, but routine diff inspection is not required.

The host prompt supplies capabilities, output limits, and permission boundaries.
The configured commit prompt owns resolution checks and announcement wording.
The GitHub skill owns issue creation and approval. The collection's `MEANS.md`
owns memory structure, retrieval, and receipts. These instructions refer to each
other instead of maintaining separate copies of the same workflow.

### Issue-resolution follow-ups

Klaus can use its existing Means collection to connect an issue to the Chatto
thread where it was reported. Enable this in the existing announcement table:

```toml
mcp_servers = ["github", "memory_klaus"]
thread_replies = true
```

Use the supplied commit prompt and GitHub skill with the Klaus collection's
`MEANS.md`. The memory connection needs `info`, `list`, `search`, `read`, and
`create`; maintenance tools remain admin-only. Optional `issue_origin` and
`issue_notification` schema fields preserve compatibility with existing entries.
After confirmed, owner-authorized issue creation, the conversation agent records
the repository and issue number with the host-supplied server, room, and thread
identifiers. It does not publish these private identifiers to GitHub. Existing
issues do not get origins automatically.

On each accepted push, the announcement agent checks those memory records and
uses GitHub read tools to verify explicit issue resolution by a merge or commit
included at the push's immutable `after` revision. A later push can therefore
send a missed notice for an earlier fix. These older fixes do not enter the
current channel announcement. Similar text, an issue mention, or closure alone
is not enough.
The agent posts a short fix notice in the recorded original thread, then creates
a separate memory receipt with the returned message ID. The announcement session
owns this receipt; conversation acknowledgments do not create another one. The
origin is unchanged.
No second memory collection or follow-up database is used. Destructive memory
actions still require explicit owner approval. Release events do not send these
follow-ups; a fix on the configured branch does not establish release availability.

The operator setting grants standing permission for these thread notifications.
The host exposes `reply_to_thread` only in opted-in push sessions and rejects
missing roots or replies passed as roots. Chatto permissions still apply. The
agent selects the origin and verifies resolution under prompt instructions;
the host does not independently validate the memory-to-issue association. Use
only trusted collection writers. Each send reuses an idempotency key derived from
the repository, origin memory ID, and destination. Memory receipts help suppress
repeats after later pushes; posting and recording a receipt are separate actions.
A lost receipt can cause a retry. Chatto deduplicates the same key and exact
message arguments for 30 minutes; different text with that key conflicts during
that window. Memory receipts remain the check for later pushes.
No historical pushes are replayed when this option is enabled.

Memory context goes to the configured model provider. Notifications go only to
the recorded thread, not to a DM or new channel root. Avoid including private
conversation details in general announcements or GitHub writes.

Klaus's prompt starts the root message with **✅ Just merged:** followed by a
15–30 word sentence that names the affected projects and the main benefit. It
targets 80–140 words in the reply, focused on observable behavior, practical
impact, and required actions. Internal symbols and file inventories are omitted
unless a public API or configuration change makes them necessary for readers.
The supplied Sweet Release notes establish intent and project scope. Klaus does
not repeat unrelated pending notes or
announce consumed release notes as new implementation changes.
The reply uses **What changed**, **Impact** when useful, and **Limitations** when
needed. It does not add routine disclaimers about uninspected diffs. Research and
editorial format remain agent instructions; the host enforces only the output
contract and length limits.

The webhook supplies the first 50 commit messages, each limited to 2,000
characters. Klaus uses these as secondary context and bases the announcement
on the supplied notes. It must not claim code inspection, successful tests, or
release availability from commit messages.

GitHub sends the webhook payload to your endpoint and receives the agent's tool
requests. The configured model provider receives the event and tool results,
which can include private source code, patches, and personal information from
GitHub. Use this feature only with a provider and destination suitable for that
data. The configured Chatto server receives the final announcement and link.
These connections reveal the Botto host's network address to the services.
Room members can read the summary, including summaries of private repositories.
Tool activity is recorded by Runling; announcement sessions use temporary storage
and do not reuse conversation transcripts.

The queue and receipts are saved in `announcements-<hash>.json` under
`runtime.data_directory`. The ledger identity includes the Chatto server and bot
account, repository, branch, and room. Run one Botto process per data directory.
Keep these files when upgrading or restarting. They contain commit text, release notes, and the
posted message body; protect them like the bot's other private state.

Confirmed announcement roots also register Botto as a participant in the thread.
Untagged human follow-ups can then reach the conversation agent without a new
mention, including after restart. Normal author checks, allowlists, and explicit
reply-recipient rules still apply. The agent can remain silent when appropriate.
Participation markers are stored under `runtime.data_directory/participation`;
keep them with the other runtime data. Startup registers existing confirmed
announcement receipts too, without reposting messages. Uncertain root sends
remain held until their receipt is resolved.

Repeated delivery of the same signed before/after commit pair does not create
another post. Queued work resumes after restart. Summary generation failures
retry after 30 seconds. Before a write, Botto saves the exact body and a stable
idempotency UUID for each message. A successful root write records its Chatto
message ID before the reply starts. Each reply has its own status, idempotency
key, and message ID. A restart after a saved root receipt resumes only the queued
reply. Existing single-message receipts remain complete and do not gain replies.

If the root send result is uncertain, `status` stays in `sending`. If the reply
result is uncertain, `detailsStatus` stays in `sending` and the root receipt stays
intact. Botto reports
that operator review is required and does not retry the write automatically.
This includes a restart between recording the send intent and saving the receipt.
Chatto's finite idempotency window cannot prevent duplicates after an unlimited
outage. This policy can therefore hold a message that was never sent.

For recovery, stop Botto and check the destination against the saved body.
For an uncertain root, set `status` to `sent` and save the actual `messageId` if
it exists. For an uncertain reply, inspect the thread under that root; set
`detailsStatus` to `sent` and save `detailsMessageId` if the reply exists. Reset
only the uncertain stage to `queued` after confirming that it was not delivered.
Do not reset a confirmed root to retry a reply.
Preserve its body and idempotency key. Back up the ledger before editing it.
Do not delete receipts to clear an error: this allows old webhook deliveries to
post again. Receipts are retained without automatic pruning.

## Automatic Cloudflare Tunnel

Botto can own the tunnel connector so a separate terminal command is not needed.
Install `cloudflared` and add these fields to `[announcements.github]`:

```toml
cloudflared_command = "/absolute/path/to/cloudflared"
cloudflare_tunnel_token = "${BOTTO_CLOUDFLARE_TUNNEL_TOKEN}"
```

Save the scoped tunnel token in the local `.env` file as
`BOTTO_CLOUDFLARE_TUNNEL_TOKEN`. This token is separate from the GitHub webhook
secret. The connector uses the token through its environment, not command-line
arguments. It does not receive the bot's other credentials. Normal startup does
not require a Wrangler login or a Cloudflare account API token.

Botto starts the connector after the webhook listener is ready. It restarts a
connector that exits, with delays from one to 30 seconds. Shutdown and configuration
reload stop the connector and wait for it to exit. A connector that does not exit
within five seconds is terminated. Connector output is suppressed because it can
contain request data; Botto logs process starts and restart attempts. A process
start does not prove that the public route is reachable.

Configure the tunnel's remote ingress to match only the webhook hostname and
`^/github$`, with service `http://127.0.0.1:5152` and a final `http_status:404`
rule. The hostname's CNAME to `<TUNNEL_ID>.cfargotunnel.com` must be **Proxied**
(orange cloud). A DNS-only record does not expose the tunnel. The console on port
5151 needs no route. Cloudflare receives the webhook payloads and the connector's
network address.

Run Botto with its normal start or development command. Keep the remote ingress
port in sync if you change the local listener port. Botto does not change DNS or
remote tunnel configuration during startup. If you omit `cloudflare_tunnel_token`,
Botto leaves connector management to you. To disable automatic startup, remove
both connector fields from the configuration.

### Failure diagnostics

Host failures name the stage: GitHub lookup, model setup or turn, response
validation, Chatto connection, thread lookup, or root/details/thread delivery.
Known transport codes can add a fixed hint, such as connection refused. External
error messages, request URLs, tokens, and message contents are not included.
Nested failures keep the original stage. Retry and uncertain-delivery handling
remain unchanged.

MCP failures remain visible in Runling's tool activity. For example, a failed
`memory_klaus` create call after a thread reply identifies a failed receipt write;
it is not reported as a failed Chatto send. Check that sequence when diagnosing
follow-ups. The model can recover from a tool error, so a failed tool event does
not necessarily mean the whole run failed.
