# GitHub commit announcements

Botto can post a short root message to a configured Chatto room after a GitHub
push, then add details as a thread reply.
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
the same secret, and the **push** event. Keep SSL verification enabled.
The listener supports GitHub's signed ping request.

The listener verifies the SHA-256 signature over the original request bytes.
It accepts bodies up to 1 MiB. Only matching repository and branch pushes enter
the queue. Branch deletions and unchanged heads do not produce announcements.
A successful response is `202`; the queue is saved before this response.
Retry failed webhook deliveries through GitHub after the listener is available.

## Content and delivery

A separate model session writes the announcement. Botto passes the repository,
branch, immutable before/after commit IDs, and commit messages to it. Botto does
not fetch diffs or prescribe a research method. The operator's prompt and selected
MCP tools guide the agent's research and writing.

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

Klaus's [announcement prompt](prompts/commit-announcement.md) tells it to inspect
patches with GitHub's `get_commit` tool using `detail: "full_patch"`. Add
`get_commit` and `list_commits` to the GitHub connection's `tools` list, alongside
`get_file_contents` for context. The agent finishes with the `finish_announcement`
tool, which supplies a channel summary and separate thread details. The host
requires a single-paragraph summary of at most 400 characters and details of at
most 4,000 characters. Intermediate model text is not posted. The host appends a
comparison link (or a commit link for a new branch) to the root summary, then
posts the details under that root message ID.

Klaus's prompt targets 25–40 words in the summary and 100–180 words in the reply.
The reply uses **What changed**, **Impact** when useful, and **Limitations** when
needed. It requires disclosure if the agent cannot read the diff. Research and
editorial format remain agent instructions; the host enforces only the output
contract and length limits.

The webhook supplies the first 50 commit messages, each limited to 2,000
characters. The prompt tells Klaus to use the immutable `after` SHA when it needs
to inspect missing history and to disclose incomplete evidence. A commit-message
summary must not claim code inspection, successful tests, or release availability.

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
Keep these files when upgrading or restarting. They contain commit text and the
posted message body; protect them like the bot's other private state.

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
