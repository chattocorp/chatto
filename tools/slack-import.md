# Import a Slack export

Import a Slack ZIP into a **blank Chatto instance** through its local operator
socket. Python 3.11 or later is required; there are no external dependencies.
The server must include the historical thread and reaction APIs (#483 and #478).
Update every server replica before import.

```sh
mise x -- python3 tools/slack-import.py path/to/export.zip \
  --operator-socket /absolute/path/to/operator.sock
```

There is no plan step or mapping file. The script reads and validates the export,
creates the required users and channels, then imports memberships, messages,
threads, and reactions. It prints counts only. No Slack downloads occur.

For an export with unavailable attachments and canvas metadata, explicitly accept
these omissions:

```sh
mise x -- python3 tools/slack-import.py path/to/export.zip \
  --operator-socket /absolute/path/to/operator.sock \
  --allow-missing-files --skip-canvases
```

## Destination and identity

Use a new instance with only its bootstrap accounts and default `general` and
`announcements` channels. Keep it idle during import. Other channel names are
rejected. The operator API does not identify bootstrap accounts or prove that
default channels have no history; using a blank instance is a prerequisite.

Imported accounts use a sanitized source login with a stable source-ID suffix.
They have no imported passwords, emails, or roles. The script creates active
human accounts needed for memberships, authorship, reactions, and mentions.
Unused accounts are skipped. Referenced bots or deactivated accounts stop the
import. Account activation remains a separate operator action.

Imported channels use `slack-` followed by the source name, so bootstrap channels
remain separate. Their descriptions combine the source topic and purpose.
Archived channels and metadata that exceed Chatto limits stop the import.

## Resume

Repeat the same command. Progress is stored automatically in
`export.zip.chatto-import/` beside the ZIP. Keep this directory: it contains the
internal source-to-destination IDs and prevents duplicate message imports.
It has mode `0700`; its journal has mode `0600`. Pending requests and recovery
records can contain private message content. Keep the ZIP and progress directory
outside version control.

The journal binds to the archive, socket, and omission options. It records the
initial destination IDs and rejects missing or unexpected accounts and rooms on
resume. A restored clone with identical IDs cannot be distinguished. Do not move
the socket to a different server or delete the journal to retry an import.

User and room creation are resumable: after a lost response, the script looks up
the pending deterministic login or channel name and records its ID. If absent,
it retries creation. Existing names without a matching pending operation are
never adopted. Membership and reaction writes can also be retried safely.

Message imports do not yet have server-side idempotency keys. If a message
response is lost, the script stops instead of risking a duplicate. Inspect the
pending request in `journal.json` and the destination message through an
authenticated read API. Verify room, author, timestamp, body, and thread.

For this exceptional case, `--reconcile resolution.json` records a reviewed
result and exits. The resolution contains `pending_sha256` (the result of
`slack_apply.fingerprint(journal["pending"])`) and either
`"response": {"messageId": "RECOVERED_ID"}` or
`"confirmed_not_written": true`. Only use the latter when inspection proves that
the write did not occur; a timeout is not proof. Then repeat the normal command.

## Current scope

- Public-channel exports described by `channels.json`, up to 256 MiB uncompressed.
- Exact author and microsecond timestamp preservation, explicit thread placement,
  and mapped reaction membership. Slack `hugging_face` becomes Chatto `hugs`.
- Rich-text sections with text, simple styles, user mentions, HTTP(S) links, and
  emoji. Unsupported formatting, legacy mrkdwn, blocks, or incomplete history
  stop the import before data writes.
- `--allow-missing-files` inserts an unavailable-file notice for each attachment;
  `--skip-canvases` omits canvas content. Avatars are always omitted. The final
  summary records all these omissions.

See [FDR-028](../docs/fdr/FDR-028-operator-api-and-cli.md) for the operator API.
Automatic recovery of uncertain message writes, broader formatting support, and
file acquisition remain separate work.

## Tests

```sh
mise x -- python3 -B -m unittest discover -s tools -p 'test_slack*.py'
```

Tests use synthetic exports and cover source validation, conversion, thread and
reaction placement, restart recovery, duplicate prevention, and destination
checks. Private exports are never repository fixtures.
