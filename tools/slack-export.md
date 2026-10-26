# Convert Slack history to a Chatto archive

Use `slack-export.py` to convert a Slack ZIP or extracted directory into the
[draft interchange format](../docs/interchange/v1.md). The command writes users,
channels, events, and original file bytes. It does not connect to Chatto.
Python 3.11 or later and `uv` are required. Run from the repository checkout:

```sh
mise x -- uv run --script tools/slack-export.py /path/to/slack-export \
  --output /path/to/new-chatto-archive
```

The output parent directory must exist. The output directory must not exist.
The converter downloads exported files by default. It uses the existing
[Slack file cache](slack-import.md#import-files), including its HTTPS host
restrictions, size limits, bounded retries, and checksum validation. Use
`--max-file-mib` and `--max-download-mib` to change the default limits of 25 MiB
per file and 1024 MiB of download traffic per run. Cached files are reused.
It does not use the Slack app token to download files.

The converter reads source metadata in place. It never extracts ZIP entries.
Metadata parsing uses memory, with a default uncompressed source limit of
256 MiB; change it with `--max-source-mib`. Original files are copied from the
download cache to the archive in bounded chunks. Each unique Slack file gets
one asset record and one local file, even when several events reference it.
File names inside the archive are safe hashes. Asset records retain the original
file name, media type, size, and SHA-256. Download URLs and tokens are not copied
into asset records.

## Keep private data on its source volume

Choose an output path on the required volume. For an extracted source, put
output below its reserved `.chatto-import/` directory. This prevents the next
conversion from reading its own output as new Slack source data:

```sh
mkdir -m 700 /path/to/slack-export/.chatto-import
mise x -- uv run --script tools/slack-export.py /path/to/slack-export \
  --output /path/to/slack-export/.chatto-import/archive-v1
```

Do not run `mkdir` if that cache directory already exists. For ZIP input, the
download cache is in `export.zip.chatto-import/` beside the ZIP. No source data
is written to the system temporary directory. Output directories use mode
`0700`; output files use `0600`. Keep both the cache and output outside version
control. They contain private data.

The command validates content before downloads and validates generated records
against local schemas before output. It writes `manifest.json` last, after all
metadata and file bytes. A failed or interrupted write leaves incomplete output;
do not import it. Inspect or remove that output yourself and retry with a fresh
output path. Download cache entries can still be reused. An existing output is
never overwritten. Identical source data, account enrichment, and conversion
results produce the same archive identity and bytes.

## Identity and optional API access

Source user IDs, usernames, display names, emails, bot status, and deactivation
state are retained. The converter does not rename accounts or merge identities.
Chatto performs email matching during import. Source channel names, visibility,
topic, purpose, and archived status are retained separately.

Use `--slack-token-file PATH` or `--slack-token-stdin` to add missing emails from
Slack. See [Slack app setup](slack-import.md#obtain-account-emails-from-slack).
The existing workspace, user-ID, status, and email conflict checks apply.
Exported emails are retained even without API access. Chatto's operator importer
trusts these emails as verified claims, so use only a trusted source archive.

Optional API access contacts `slack.com` for `auth.test` and `users.list`.
Default file downloads contact only `files.slack.com` and
`files-origin.slack.com`. Slack receives the requests and the machine's public
IP address. File URL credentials go only to those file hosts; no bearer token
or cookies are sent for downloads. No Chatto-operated service receives data.
`uv` can contact the Python package registry to install the pinned validator
and its dependencies. Schema validation itself makes no network requests.

## History and omissions

Slack supplies final message bodies and reaction sets. The converter writes
`message.snapshot` records with known creation/edit times and unknown deletion
times. Tombstones contain no body. Thread replies keep their root reference;
the first reply establishes a thread without inventing its creation time.
Known join/leave, name, topic, purpose, archive, and unarchive records become
native interchange events. Channel changes require structured source fields;
the converter does not extract values from human-readable notice text.
Every channel ends with an authoritative membership snapshot.

The manifest marks history as partial and capture consistency as unknown.
Known gaps are declared as omissions. It does not use the conversion time as
the source capture time. Rich-text conversion keeps the existing supported
Markdown subset. Rich-text user mentions become literal names because the draft
format has no structured mention contract. Custom or unknown reaction emoji,
unresolved references, unsupported blocks, and ambiguous history stop conversion.

- `--allow-missing-files` writes unavailable asset records when files cannot be
  downloaded. It does not invent placeholder message text.
- `--skip-files --allow-missing-files` skips all downloads and records files as
  unavailable.
- `--allow-attachment-summaries` retains supported attachment text and links,
  with a declared loss of layout and embedded media.
- `--skip-canvases` declares the loss of canvas content.

Avatars, integration logs, and thread-broadcast channel echoes are declared
omissions. Missing exported replies and incomplete reactor lists are reported
as history omissions. No missing users or thread roots are fabricated.

## Import the result

```sh
chatto operator import /path/to/new-chatto-archive --validate-only
chatto operator import /path/to/new-chatto-archive --accept-omissions
```

The converter can represent more data than the initial native importer accepts.
In particular, that importer currently rejects bots, deactivated accounts,
private channels, unavailable files, and files shared by several messages.
It also limits an archive to 64 MiB, 20,000 records, and 1 MiB per record.
An export that exceeds these limits is not truncated. See the
[current import limits](../schema/interchange/README.md) before loading it.
Channel and username decorations belong to `chatto operator import`.

The legacy [direct Slack importer](slack-import.md) remains available for
existing runs and journals. Its operator calls and resume policy are unchanged.
There is no conversion of its journal into native import progress.

Run `mise test-slack` and `mise test-interchange` for synthetic tests. Never add
private Slack exports as repository fixtures. Run `mise codegen-emoji` after
updating the bundled emoji data; it produces the backend map and the converter's
JSON map from the same source.
