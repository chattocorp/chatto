# Chatto interchange format

**Status: draft v1.** This directory defines a portable JSON format for channel
history. It is not a raw EVT dump or a server backup. Use
`chatto operator import PATH --validate-only` to check the archive and destination
through the operator socket. Omit the flag to import. Export is not implemented
yet. The existing Slack importer is unchanged.

Start with the [converter guide](../../docs/interchange/converter-guide.md),
then read the [v1 specification](../../docs/interchange/v1.md).
The [conversation example](v1/examples/conversation/) contains synthetic data
only. The schemas and examples are Apache-2.0 licensed.

Run the contract tests with `mise test-interchange`. Tests validate the schemas,
examples, reference rules, and representative invalid archives. They do not
connect to Chatto or prove destination compatibility. Development dependency
installation can contact the Python package registry; fixture validation itself
uses only local schemas and files.

The first importer accepts public channels and active human users. New accounts
require usernames. Message creation times must be known. Unavailable content
and files are not yet supported; tombstone snapshots are supported. Each file
must belong to one message, although its edits can reference the same file.
Original file bytes are retained; derivative media processing is not started.
These restrictions fail destination validation before any writes.

Use `--channel-prefix`, `--channel-suffix`, `--username-prefix`, and
`--username-suffix` to decorate destination names. Defaults preserve source
names. Collisions and invalid destination names fail validation. Reuse exactly
the same archive and naming options when resuming. Declared omissions require
`--accept-omissions` for writes. Import creates no passwords.

The root-equivalent importer trusts source email claims. It trims whitespace
and compares addresses case-insensitively using Chatto's email ownership index.
An owned email reuses its existing account and preserves that account's profile,
username, and credentials. Unowned emails are attached as verified addresses.
They can be used by the server's configured email sign-in flow. Source records
connected by shared emails resolve to one account; the first record supplies
the profile for a new account. Emails pointing to different existing accounts
fail validation. Names alone never cause account matching.

The initial destination can have existing accounts and empty channels, but
no messages. Keep it unused until import completes.
Unrelated durable writes stop an unfinished import. One archive is bound to the
destination; a completed import can be repeated without creating duplicates.
Progress has no expiry and is stored on the server. No local journal is created.

The Go reader checks the same schemas, references, final snapshots, deletion
privacy, and attachment hashes. It rejects links and extra files. This first
reader accepts at most 64 MiB across metadata and files, 20,000 JSON records
(including the manifest), and 1 MiB per record. These are implementation
limits, not format limits. Larger inputs fail; they are not truncated.

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
