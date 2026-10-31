# Chatto interchange format

**Status: draft v1.** This directory defines a portable JSON format for channel
history. It is not a raw EVT dump or a server backup. Use
`chatto operator import PATH --validate-only` to check the archive and destination
through the operator socket. Omit the flag to import. Export is not implemented
yet. The [Slack converter](../../tools/slack-export.md) produces this format;
the legacy direct Slack importer remains available for existing runs.

Start with the [converter guide](../../docs/interchange/converter-guide.md),
then read the [v1 specification](../../docs/interchange/v1.md).
The [conversation example](v1/examples/conversation/) contains synthetic data
only. The schemas and examples are Apache-2.0 licensed.

Run the contract tests with `mise test-interchange`. Tests validate the schemas,
examples, reference rules, and representative invalid archives. They do not
connect to Chatto or prove destination compatibility. Development dependency
installation can contact the Python package registry; fixture validation itself
uses only local schemas and files.

The importer accepts public channels and human or bot accounts, including
deactivated accounts. New human accounts require usernames. Bots without a
username receive `bot-` followed by 24 hexadecimal characters derived from the
source ID. Supplied names are preserved. Message creation times must be known. Unavailable content
and files are not yet supported; tombstone snapshots are supported. Shared
files are uploaded separately for each message. Original file bytes use the
normal upload API and configured media processing. Record support is checked
before writes; server media and account policies still apply to each API call.

Use `--channel-prefix`, `--channel-suffix`, `--username-prefix`, and
`--username-suffix` to decorate destination names. Defaults preserve source
names. Collisions and invalid destination names fail validation. Reuse exactly
the same archive and naming options when resuming. Declared omissions require
`--accept-omissions` for writes. Import creates no passwords.

Bots belong to the destination's active human server owner by default. If there
are multiple owners, select one with `--bot-owner USERNAME`. This optional
override can also select another active human. The selected ID is saved in the
journal before account creation and reused on retries. Imported bots receive no API
keys or webhooks and cannot have email addresses. Initial deactivation commits
with account creation. Deleted accounts remain unsupported. Older servers
without `ImportUser` reject these new account types; they never substitute active
humans. Active-human imports retain the existing creation API and retry keys.

The root-equivalent importer trusts source email claims. It trims whitespace
and compares addresses case-insensitively using Chatto's email ownership index.
An owned email reuses its existing account and preserves that account's profile,
username, credentials, and activation status. Unowned emails are attached as verified addresses.
They can be used by the server's configured email sign-in flow. Source records
connected by shared emails resolve to one account; the first record supplies
the profile for a new account. Emails pointing to different existing accounts
fail validation. Shared source identities with different kinds or statuses also
fail validation. Names alone never cause account matching.

The importer creates new channels and never merges existing channel history.
Optional user `avatar_asset_id` references local image bytes in the asset catalog.
The importer accepts avatar images up to 5 MiB and fills only empty avatars,
including on email-matched accounts. It preserves existing avatars and records
skipped assignments so retries cannot undo a later avatar clear. Servers without
the new avatar operation reject the request; no remote URLs are sent to Chatto.
Older draft readers can reject the optional avatar field during schema validation.
Archives without that field retain their existing behavior.

Keep imported channels unused until completion. The CLI stores a private local
journal in the user cache directory; `--progress PATH` selects another file.
Keep that journal and the unchanged archive for resume. It binds source bytes,
naming options, and the operator socket path. Server operation receipts prevent
duplicates after lost responses. Completed imports can be repeated with the
same journal without writes. Old server-side archive progress cannot be resumed
by the new CLI. Do not point an existing journal at a replacement server.

The Go reader checks the same schemas, references, final snapshots, deletion
privacy, and attachment hashes. It rejects links and extra files. Event bodies
and file bytes are streamed; catalogs and reference indexes use CLI memory.
There is no total byte or record-count limit. A JSON record is limited to 1 MiB.
Normal server upload expiry and per-file limits remain in effect. The server
receives typed resource calls, not archive bytes or source paths.

From `cli/`, run `mise x -- go test ./internal/interchange` to check the reader.
After a schema change, run `mise x -- go generate ./internal/interchange` to
update its embedded schema copy. A test checks that this copy matches the
authoritative files. Validation does not fetch the schema URLs.

The six `*.schema.json` files are the authoritative structural definitions.
Each property has the meaning specified in the written specification. The
`$id` URLs identify schemas; they are not currently hosted download endpoints.
Resolve them from this directory, without network retrieval.

Changes to this draft can still break compatibility. Do not use draft archives
as the only copy of source data. Before v1 is released, freeze its schemas and
semantics and publish the schemas at stable locations. A released version must
not acquire new required fields, event types, or changed meanings in place.
