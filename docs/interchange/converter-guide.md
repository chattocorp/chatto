# Write a Chatto interchange converter

The format is a draft. The initial generic importer is available; export remains
planned. Check the [supported import subset](../../schema/interchange/README.md).
Build against the [specification](v1.md), not Chatto's
internal EVT protobufs or the current Slack operator script.

1. Read the source in place. Preserve a known source capture time and consistency
   guarantee. If absent, use null capture time and unknown consistency; declare
   the uncertainty. Stop if required source files contradict each other.
2. Write source users and channels to the catalogs. Preserve source IDs and
   names. Include former members and deleted/bot placeholders when history
   references them. Preserve source email claims; let the destination importer
   resolve account matches rather than replacing source IDs in the converter.
3. Write events in causal order. Preserve dated joins, leaves, posts, edits,
   reactions, and room changes when available. IDs must be deterministic within
   this immutable export; keep message IDs separate from operation event IDs.
4. Use a final message snapshot for current text, reactions without timestamps,
   and tombstones without a deletion time. Do not fabricate missing operations.
   Mark history partial and explain the gaps in the manifest.
5. Write one final membership snapshot per room, even when full join/leave
   history exists. This set controls final membership; it creates no join notices.
6. Convert text to Chatto Markdown. Copy available attachment bytes into `files/`
   using safe names. Keep original names in asset metadata. Hash the actual bytes.
   Explicitly record unavailable files; do not put credentials or download URLs
   into the archive. Ask the operator to choose the output location; never copy
   private source data to an implicit temporary directory.
7. Declare all known omissions. Validate JSON Schema with format checks enabled,
   then validate references, ordering, final state, and files as specified.

Start from the [synthetic conversation archive](../../schema/interchange/v1/examples/conversation/).
It shows real join/leave events, a threaded reply and edit, a reaction, a local
attachment, and a tombstone whose original content and deletion time are unknown.
The final membership snapshot contains only the remaining member.

After a converter produces an archive, the command flow is:

```sh
chatto operator import ./converted --validate-only
chatto operator import ./converted --accept-omissions
```

The [Slack converter](../../tools/slack-export.md) produces this format with
local file bytes. It needs no operator socket. Its output is also readable by
an archive reader or another platform's importer. The legacy direct Slack
importer remains available for existing runs.

Run `mise test-interchange` to check the repository's schemas and synthetic
fixtures. That is a development conformance suite, not a validator for a private
export. Do not add real exports to its fixture directory.

The schemas use [JSON Schema Draft 2020-12](https://json-schema.org/draft/2020-12/json-schema-validation).
Enable `format` validation explicitly; many validators otherwise treat dates,
email addresses, and UUID formats as annotations. Schema validation does not
verify references or prove that Chatto can represent a source record.
