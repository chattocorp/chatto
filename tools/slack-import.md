# Import a Slack export

For new migrations, use the [portable Slack converter](slack-export.md), then
`chatto operator import`. This direct importer remains available for existing
runs and recovery journals.

Import a Slack ZIP or extracted directory into a **blank Chatto instance** through its local operator
socket. Python 3.11 or later is required; there are no external dependencies.
The server must support historical threads, reactions, and import idempotency keys.
Update every server replica before import.

```sh
mise x -- python3 tools/slack-import.py path/to/export.zip \
  --operator-socket /absolute/path/to/operator.sock
```

There is no plan step or mapping file. The script reads and validates the export,
creates the required users and channels, then imports memberships, messages,
threads, and reactions. It prints counts only. By default, it does not contact
Slack. Optional Slack API access supplies account email addresses. A separate
`--download-files` option downloads exported files. Avatars are not imported.

## Inspect before import

Use `--preflight` without an operator socket to inspect the source without
contacting Chatto. The script reads files in place; it does not copy or extract
them. By default, it writes no files. Add `--save-report` to save aggregate counts
and conversion errors in the progress directory. Reports contain no message
content, user names, source IDs, or channel names.

```sh
mise x -- python3 -B tools/slack-import.py /path/to/extracted-export \
  --preflight --save-report --max-source-mib 512
```

The default source limit is 256 MiB of uncompressed data. Increase it explicitly
for larger exports. Parsing keeps source records in memory and can use more RAM
than the source size. Directory input rejects symbolic links and special files.
Keep the source unchanged while the command runs.

Preflight counts the first conversion error per failed message. Conversion
assumes file placeholders and attachment text summaries so other content gaps remain visible. This does
not approve missing files or prove that an import can complete: user policies,
source gaps, and destination checks still apply. Exit code zero means inspection
completed, even when the report lists gaps.

For an export with unavailable attachments and canvas metadata, explicitly accept
these omissions:

```sh
mise x -- python3 tools/slack-import.py path/to/export.zip \
  --operator-socket /absolute/path/to/operator.sock \
  --allow-missing-files --skip-canvases --allow-attachment-summaries
```

## Destination and identity

Use a new instance with only its bootstrap accounts and default `general` and
`announcements` channels. You can rename bootstrap universal channels to free
their names for imported channels. Keep the instance idle during import. Other
non-universal channels are rejected. The operator API does not identify bootstrap accounts or prove that
default channels have no history; using a blank instance is a prerequisite.

Imported accounts keep the export's username (`users.json` → `name`), including
periods. No source-ID suffix is added. Invalid usernames and case-insensitive
collisions stop import before user creation; the importer does not rename users
or merge accounts. Display names still come from the Slack profile.
They have no imported passwords or roles. Emails are imported only with the
Slack API option described below. The script creates active
human accounts needed for memberships, authorship, reactions, and mentions.
Unused accounts are skipped. Referenced bots or deactivated accounts stop the
import. Account activation remains a separate operator action.

Imported channels keep their source names. Use `--channel-prefix slack-` if you
want a prefix. A source name that conflicts with a bootstrap channel stops
import; prepare the blank instance so the names are available. Renamed bootstrap
universal channels are accepted. The importer does not adopt an existing
channel by name. Channel descriptions combine the source topic and purpose.
Archived channels and metadata that exceed Chatto limits stop the import.

## Obtain account emails from Slack

If the export omits account emails, use a temporary Slack app to read them.
Open [Slack's app creation page](https://api.slack.com/apps?new_app=1), select
**From a manifest**, select the source workspace, and paste
[`slack-app-manifest.yaml`](slack-app-manifest.yaml). Create the app, then select
**OAuth & Permissions → Install to Workspace**. The manifest requests only
`users:read` and `users:read.email`.

Save the **Bot User OAuth Token** in a private file outside version control.
For directory input, put it inside the source's `.chatto-import/` directory so
it is excluded from source inspection. Set the file mode to `0600`. Do not put
the token in a command argument or paste it into logs.

```sh
mise x -- python3 -B tools/slack-import.py /path/to/extracted-export \
  --preflight --slack-token-file /path/to/extracted-export/.chatto-import/slack-token
```

Add the same `--slack-token-file` option to the import command. Alternatively,
use `--slack-token-stdin` to read the token from standard input. The two options
are mutually exclusive. Neither option saves the token or the API directory.
Preflight prints aggregate identity coverage, without email addresses or IDs.

API access sends authenticated HTTPS requests to `slack.com` for `auth.test`
and `users.list`. Slack receives the importing machine's public IP address and
the API requests. No data passes through a Chatto-operated service. Redirects
are refused. Rate-limit and server-error retries are bounded.

The token must identify the single workspace recorded in the export. Users are
matched by Slack user ID, never by name or email. Every active human in the
export must have a matching API user and a unique, usable email. A different
workspace, changed active-account status, conflicting export email, or duplicate
email stops processing before destination writes. Bots and deactivated users
receive no email; their existing import restrictions still apply.

The importer adds each needed account's email as verified through the operator
API. An email already assigned to a different destination account stops import;
the importer does not merge accounts. It creates no passwords and sends no
invitations or password-reset emails. Configure and verify account activation
separately before migration. Delete the temporary Slack app and token file when
the import and its verification are complete.

## Import files

Add `--download-files` to download files from the URLs in the export and attach
them to their original messages through the operator API. This also supports
messages that contain only files. File downloads do not use the Slack app token.
The export URLs must still grant access to the files.

Inspect availability before importing:

```sh
mise x -- python3 -B tools/slack-import.py /path/to/extracted-export \
  --preflight --download-files --max-source-mib 512
```

Unlike ordinary preflight, this command makes network requests and saves files.
It downloads each unique Slack file once into the progress directory's `files/`
cache. For directory input, this cache stays inside the source directory.
Downloads stream to private temporary files in that cache, then move into place
after size and checksum checks. Subsequent runs verify and reuse cached bytes.
Cache metadata contains no signed URLs. Keep the cache outside version control.

Downloads contact only `files.slack.com` and `files-origin.slack.com` over HTTPS,
including redirects. Slack receives the URL credentials and the importing
machine's public IP address. No app token or cookies are sent. Rate-limit and
server-error retries are bounded. Remote files on other services are not fetched.

The default limits are 25 MiB per file and 1024 MiB of download traffic per run,
including retries. Use `--max-file-mib` and `--max-download-mib` to change them.
The destination's upload limits also apply. Chatto accepts at most ten files per
message. Messages that exceed this limit stop the import.

Hidden, deleted, inaccessible, external, and oversized files appear as aggregate
failure counts. Without `--allow-missing-files`, unavailable files stop import
before destination writes. With that option, their messages retain explicit
unavailable-file notices. Slack attachment layouts still require
`--allow-attachment-summaries` where applicable.

Uploads use the existing operator asset API. Completed uploads are recorded
before the message is sent. After an interrupted upload, resume cancels a known
unfinished session and starts it again; an uncertain completion is retried.
A lost session-creation response can leave a session for server cleanup.
Unattached completed assets are reused for up to 23 hours, then uploaded again.
Message recovery always uses the original attachment IDs and the normal bounded
idempotency window described below.

File contents and omissions become part of the resume fingerprint. Do not add
or remove `--download-files` during an import. If file availability changes,
resume stops for review rather than silently changing the imported messages.
This option does not add files to messages from an earlier import.

The destination stores uploaded file content. If the source must stay on a
specific volume, put the destination's data directory and `TMPDIR` on that volume
as well: server-side asset processing can use temporary files.

## Resume

Repeat the same command. Progress is stored automatically in
`export.zip.chatto-import/` beside the ZIP. Keep this directory: it contains the
internal source-to-destination IDs and prevents duplicate message imports.
It has mode `0700`; its journal has mode `0600`. New runs use `journal.sqlite`
with incremental transactions and full synchronous writes. Completed results
are not serialized again for each operation. A matching legacy `journal.json`
is migrated on first use; after migration, SQLite is authoritative. Keep the
whole progress directory, including any SQLite WAL files. Pending requests and recovery
records can contain private message content and imported email addresses. Keep the ZIP and progress directory
outside version control.

For directory input, progress and saved reports stay in `.chatto-import/` inside
the source directory. That reserved directory is excluded from source inspection.
The resume fingerprint includes source file names and bytes. Do not switch
between a ZIP and directory input during an import.

The journal binds to the archive, socket, naming policy, and omission options. It records the
initial destination IDs and rejects missing or unexpected accounts and rooms on
resume. A restored clone with identical IDs cannot be distinguished. Do not move
the socket to a different server or delete the journal to retry an import.
Journals from the earlier automatic username-suffix policy cannot be resumed
with the new naming policy. Use a fresh destination and progress directory for
the next import; keep old progress if the old destination still needs recovery.

With API access, each run fetches the directory again. The matched email mapping
is part of the resume fingerprint; changes stop resume before recovery or writes.
Additional API users outside the export do not change the fingerprint. Token
rotation does not change it either. Do not add or remove API access halfway
through an import. This option does not attach emails to an earlier offline import.

User and room creation are resumable: after a lost response, the script looks up
the pending deterministic login or channel name and records its ID. If absent,
it retries creation. Existing names without a matching pending operation are
never adopted. Email assignment is a separate journaled operation and can be
retried safely, as can membership and reaction writes. Resume checks that
completed email assignments still belong to their imported accounts.

Each message gets a UUID and first-attempt time, saved before dispatch. If its
response is lost, repeat the command: the script sends the exact saved request
and UUID and recovers the original ID. It does not refresh the first-attempt
time. Retries stop at 29 minutes, leaving a margin before the server's fixed
30-minute collision window. Keep the system clock correct; a detected backward
clock jump also stops recovery. Server support is checked before any writes.

Older journals with an unkeyed pending message and requests outside the retry
window still require reconciliation instead of risking a duplicate. Inspect the
pending request in the journal (`state.metadata` in SQLite) and the destination message through an
authenticated read API. Verify room, author, timestamp, body, and thread.

For this exceptional case, `--reconcile resolution.json` records a reviewed
result and exits. The resolution contains `pending_sha256` (the result of
`slack_apply.fingerprint(journal["pending"])`) and either
`"response": {"messageId": "RECOVERED_ID"}` or
`"confirmed_not_written": true`. Only use the latter when inspection proves that
the write did not occur; a timeout is not proof. Then repeat the normal command.

## Current scope

- Public-channel exports described by `channels.json`, with a default limit of
  256 MiB uncompressed. Use `--max-source-mib` to change the limit.
- Exact author and microsecond timestamp preservation, explicit thread placement,
  and mapped reaction membership. Slack `hugging_face` becomes Chatto `hugs`.
- Rich-text paragraphs, nested bullet and numbered lists, quotes, and code
  blocks. Code delimiters adapt to backticks in the source. Lists retain source
  numbering offsets and continuation lines; indentation without a parent stops
  import instead of guessing the missing structure.
- Inline bold, italic, underline, strike, code, user mentions, and Unicode emoji (including
  skin tones). Channel references become the imported channel name as text;
  message references retain their Slack permalink. Broadcast references remain
  code text. Text marked `unlink` also becomes code text to prevent automatic
  linking. Underline uses Chatto's `__text__` syntax, including on links and
  mentions; bold uses `**text**`.
- HTTP(S) and mail links retain labels and destinations, including parentheses
  and encoded spaces. Other URL schemes, including phone links, remain readable
  code text because Chatto does not support them as message links. No URL is
  fetched during conversion. Opening a source link can contact its external service.
- Literal punctuation uses character entities because Chatto disables Markdown
  backslash escapes. Unsupported formatting, legacy mrkdwn, Block Kit layouts,
  or incomplete history stop the import before data writes.
- Channel join, leave, rename, topic, purpose, archive, and unarchive notices
  become labeled historical text. They do not change Chatto membership or room
  state. Source markup in these notices stays literal. Actions (`me_message`)
  receive a label. Thread broadcasts stay in their thread without a channel echo.
  Bot message subtypes can be converted, but existing author and bot-account
  policy checks still apply. Slack `tombstone` records become native deleted
  messages with no stored body. Their original timestamp and thread identity
  remain, and historical replies can target them. Reactions are imported normally
  and remain visible on the tombstone. Tombstones with attachments or blocks
  stop import for review. Other deletion records and unknown
  subtypes stop import. Exports with tombstones require deleted-import support
  on all server replicas; the script checks support before writes or recovery.
- `--allow-attachment-summaries` preserves exported titles, text, fields, and
  source URLs as explicit links. Fallback text is used only when primary content
  is absent. Shared-message previews become one attributed quote with a link to
  the original Slack message. Slack markup stays literal.
  Layout, blocks, and embedded media are not reproduced. HTML is never rendered;
  remote images and media are not fetched or embedded. Source links can contact
  external services if a reader opens them. Interactive attachments and
  attachments without text stop import. The final counts include these summaries.
- `--download-files` imports available files as native message attachments.
  `--allow-missing-files` inserts an unavailable-file notice for each missing file;
  `--skip-canvases` omits canvas content. Avatars are always omitted. The final
  summary records all these omissions.

See [FDR-028](../docs/fdr/FDR-028-operator-api-and-cli.md) for the operator API.
Recovery beyond the bounded retry window, broader formatting support, and
avatar import remain separate work.

## Tests

```sh
mise test-slack
mise x -- pnpm --dir apps/frontend exec tsx ../../tools/test-slack-markdown.mjs
```

Tests use synthetic exports and cover source validation, conversion, thread and
reaction placement, restart recovery, duplicate prevention, and destination
checks. Private exports are never repository fixtures.
The optional rendering check uses the frontend's installed Markdown engine and
link policy with synthetic input. It requires the workspace development dependencies;
the importer itself still has no external Python or Node dependencies.
