# Slack import planner

`slack-import.py` inspects a Slack ZIP export and prepares source-to-Chatto ID
mappings. It requires Python 3.11 or later and has no external dependencies.
It does not connect to Slack or Chatto. It does not import data yet.

## Create a plan

Run from the repository root. Select a new output directory on each run:

```sh
mise x -- python3 tools/slack-import.py path/to/export.zip \
  --output-dir .context/slack-plan
```

The script prints counts and issue codes. It writes three files:

- `report.json`: counts, unresolved mapping counts, and detected gaps.
- `mapping.json`: source user and channel IDs with null destination IDs.
- `plan.json`: user proposals, channel metadata, ordered messages, thread
  references, reactions, file references, and canvas metadata.

The plan contains private source content, including message text and metadata.
Keep it in `.context/` or another private, untracked directory. Output directories
have mode `0700`; files have mode `0600`. Existing output directories are refused.
No export contents are added to repository fixtures.

## Map existing accounts and rooms

Edit the values in `mapping.json` to specify existing Chatto user and room IDs.
Use the operator user and room lookup commands to obtain these IDs. Null means
unresolved. It does not authorize account or room creation. The script does not
check whether a destination exists or belongs to the intended Chatto server.

```sh
mise x -- python3 tools/slack-import.py path/to/export.zip \
  --mapping .context/slack-plan/mapping.json \
  --output-dir .context/slack-plan-mapped
```

The mapping is bound to the exact archive SHA-256. The script rejects unknown
source IDs and mappings that merge accounts or rooms. A future writer must bind
the plan and its progress journal to the destination server as well.

## Current limits and next work

The planner supports public-channel message files described by `channels.json`.
Other archive entries are reported as unhandled. JSON arrays that contain an
empty string are treated as empty placeholders. The inspection limit is 256 MiB
of uncompressed archive data. The script reads JSON into memory.

Source message keys combine channel ID and exact Slack timestamp. The archive
hash scopes the plan. Messages are sorted within each channel by source time;
timestamp conversion retains all six fractional digits. Source text and rich
text blocks are retained, not converted into Chatto message bodies. A future
writer must handle formatting, mapped mentions, links, emoji, and subtypes.

Before an apply command is added, resolve these gaps:

- Use the operator message API's explicit thread placement and empty-thread
  creation. A reply reference alone does not create a thread.
- Use operator reaction import with an explicit mapped user. It preserves
  reaction membership without requiring timestamps absent from the export.
- Define recovery for a write whose response is lost. A local progress journal
  cannot prove whether such a message was stored. The current API creates a new
  message on every successful import call.
- Select policies for deactivated users, bots, avatars, archived channels, and
  channel topic/purpose conversion. Do not copy administrator roles or mark
  exported email addresses as verified automatically.
- Resolve missing files and canvas contents. `hidden_by_limit` file records do
  not supply downloadable content. Downloading other files or avatars requires
  a separate explicit step that contacts Slack or its storage provider and
  discloses the downloader's IP address. No downloads occur during planning.

Issue counts describe source occurrences, not a readiness guarantee. For example,
`thread_import_required` counts replies and
`reaction_import_required` counts exported reactor memberships. These counts
identify work for the apply stage, not missing server capabilities. Avatar
counts refer to profiles with an `image_original` URL. Missing or unsupported
records remain visible in the plan or its unhandled-entry list.

See [FDR-028](../docs/fdr/FDR-028-operator-api-and-cli.md) for the current operator
contract. The planner does not check the destination server's capabilities.

## Verification

```sh
mise x -- python3 -B -m unittest discover -s tools -p test_slack_import.py
```

Tests use synthetic ZIP exports. They check exact timestamp conversion, source
order, retained content, missing references, duplicate rejection, mapping reuse,
and private output files. They do not test a live import.
