#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["jsonschema[format]==4.26.0"]
# ///
"""Convert a Slack export into a portable Chatto archive, without a server."""

import argparse
import collections
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import uuid

from slack_apply import ApplyError, CHANNEL_NOTICES, convert, fingerprint
from slack_api import SlackAPI, enrich_users, read_token
from slack_files import FileCache, FileImports, file_inventory, file_references

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

# Keep the existing command and its recovery journals usable. Its source reader
# has no side effects on import; conversion never constructs an Operator client.
spec = importlib.util.spec_from_file_location("slack_source", Path(__file__).with_name("slack-import.py"))
source = importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)


def text(value):
    """Slack channel fields can be strings or value/creator metadata objects."""
    result = value.get("value", "") if isinstance(value, dict) else value or ""
    if not isinstance(result, str):
        raise ApplyError("Invalid channel metadata.")
    return result


def literal_mentions(value):
    """Keep mentions readable without binding to a potentially renamed account."""
    count = 0
    if isinstance(value, dict):
        if value.get("type") == "user":
            value.setdefault("style", {})["code"] = True
            count += 1
        for child in value.values():
            count += literal_mentions(child)
    elif isinstance(value, list):
        for child in value:
            count += literal_mentions(child)
    return count


def reaction_unicode(name, emojis):
    """Resolve bundled aliases and Slack skin-tone suffixes; never guess custom emoji."""
    if not isinstance(name, str):
        raise ApplyError("Invalid reaction name.")
    base, separator, tone = name.partition("::skin-tone-")
    value = emojis.get({"hugging_face": "hugs"}.get(base, base))
    if not value:
        raise ApplyError("Custom or unknown reaction emoji is unsupported.")
    if separator:
        if tone not in {"2", "3", "4", "5", "6"} or "\u200d" in value:
            raise ApplyError("Unsupported reaction skin-tone sequence.")
        value = value.rstrip("\ufe0f") + chr(0x1F3FB + int(tone) - 2)
    return value


def build_archive(plan, downloads=None, *, allow_missing_files=False,
                  allow_attachment_summaries=False, skip_canvases=False):
    """Build portable records and a file-copy list. Never call a destination API.

    Slack message rows are final observations, not edit/reaction history.
    User IDs remain source identities; the destination owns email matching.
    """
    issues = plan["summary"]["issues"]
    fatal = {"unhandled_archive_entries", "unsupported_lists", "unsupported_huddle_transcripts",
             "unsupported_file_conversations", "missing_thread_root", "invalid_thread_root"}
    if fatal.intersection(issues):
        raise ApplyError("Export contains unsupported records or missing thread roots.")
    if plan["canvases"] and not skip_canvases:
        raise ApplyError("Canvas metadata requires --skip-canvases.")
    emojis = json.loads(Path(__file__).with_name("emoji-map.json").read_text())
    omissions = [{"category": "history", "reason": "Slack supplies final message and reaction state, without a consistent capture cut or complete history.", "count": None}]

    def omit(category, reason, count):
        if count:
            omissions.append({"category": category, "reason": reason, "count": count})

    users = []
    for u in plan["users"]:
        email = u.get("verified_email") or u.get("source_email")
        users.append({"id": u["source_id"], "username": u["proposed_login"],
                      "display_name": u["proposed_display_name"] or u["proposed_login"] or u["source_id"],
                      "kind": "bot" if u["bot"] or u["source_id"] == "USLACKBOT" else "human",
                      "status": "deactivated" if u["deactivated"] else "active",
                      "emails": [email] if email else []})
    logins = {u["id"]: u["username"] or u["display_name"] for u in users}
    rooms = [{"id": r["source_id"], "kind": "channel", "name": r["name"], "topic": text(r["topic"]),
              "purpose": text(r["purpose"]), "visibility": "private" if r.get("private") else "public", "archived": r["archived"]}
             for r in plan["rooms"]]
    channels = {r["id"]: r["name"] for r in rooms}
    inventory = file_inventory(plan)
    assets, copies = [], {}
    unavailable = 0
    for key, item in sorted(inventory.items()):
        ready = downloads.ready.get(key) if downloads else None
        if ready:
            path = "files/" + hashlib.sha256(key.encode()).hexdigest()
            storage = {"status": "available", "path": path, "size": ready["size"], "sha256": ready["sha256"]}
            copies[path] = ready
        else:
            if not allow_missing_files:
                raise ApplyError("Files are unavailable; retry downloads or use --allow-missing-files.")
            unavailable += 1
            failure = downloads.failures.get(key) if downloads else None
            reason = {"file_deleted": "deleted", "file_not_found": "deleted",
                      "download_access_denied": "access_denied", "external_file": "external",
                      "hidden_by_limit": "not_exported", "no_download_url": "not_exported"}.get(failure, "unknown")
            storage = {"status": "unavailable", "reason": reason}
        assets.append({"id": key, "filename": (ready or {}).get("filename") or item.get("name") or "attachment",
                       "media_type": (ready or {}).get("content_type") or item.get("mimetype") or "application/octet-stream",
                       "storage": storage})
    omit("files", "Source files are unavailable or downloads were explicitly skipped.", unavailable)
    omit("unsupported", "User avatars are not included.", sum(u["has_avatar"] for u in plan["users"]))
    omit("unsupported", "Canvas contents are not included.", len(plan["canvases"]))
    omit("unsupported", "Integration logs are not included.", plan["ignored_integration_logs"])
    omit("history", "The source reports replies outside the exported history.", issues.get("incomplete_thread_replies", 0))
    omit("history", "The source does not identify every reactor; only known reactors are retained.", issues.get("incomplete_reaction_users", 0))
    events, introduced, membership = [], set(), {}
    mentions = summaries = broadcasts = 0
    for row in plan["messages"]:
        original = row["source"]
        subtype, key, room = original.get("subtype"), row["source_key"], row["source_channel_id"]
        event = {"id": "event:" + key, "room_id": room, "actor_id": None,
                 "occurred_at": None}
        author = row["source_author_id"]
        if subtype in CHANNEL_NOTICES:
            if original.get("blocks") or file_references(original) or original.get("attachments") or original.get("reactions"):
                raise ApplyError("Channel event contains unsupported message content.")
            event.update(occurred_at=row["created_at"], actor_id=author if author in logins else None)
            if subtype in {"channel_join", "channel_leave"}:
                if author not in logins:
                    raise ApplyError("Unresolved membership user.")
                present = subtype == "channel_join"
                member = (room, author)
                if member in membership and membership[member] == present:
                    raise ApplyError("Membership history has a repeated transition without its counterpart.")
                membership[member] = present
                event.update(type="room.member_joined" if subtype == "channel_join" else "room.member_left", data={"user_id": author})
                inviter = original.get("inviter")
                if inviter:
                    if inviter not in logins:
                        raise ApplyError("Unresolved membership actor.")
                    event["actor_id"] = inviter
            elif subtype in {"channel_archive", "channel_unarchive"}:
                event.update(type="room.archived" if subtype == "channel_archive" else "room.unarchived", data={})
            else:
                field = {"channel_name": "name", "channel_topic": "topic", "channel_purpose": "purpose"}[subtype]
                if field not in original or not isinstance(original[field], str) or (field == "name" and not original[field]):
                    raise ApplyError("Channel event lacks structured metadata; text is not guessed.")
                event.update(type="room." + field + "_changed", data={"value": original[field]})
            events.append(event)
            continue
        if author not in logins:
            raise ApplyError("Unresolved message author.")
        root = row["thread_root_key"]
        if root == key:
            root = None
        if root and root not in introduced:
            raise ApplyError("Thread root must be a message before its replies.")
        # No ten-file destination limit here: this is a portable converter.
        message = copy.deepcopy(original)
        references = list(dict.fromkeys(f["id"] for f in file_references(message)))
        if subtype == "tombstone" and references:
            raise ApplyError("Tombstone contains unsupported content.")
        message["files"] = []
        attachments = []
        for attachment in message.get("attachments", []):
            had_files = bool(attachment.get("files"))
            attachment["files"] = []
            if had_files and set(attachment) <= {"files", "id"}:
                continue
            attachments.append(attachment)
        if "attachments" in message:
            message["attachments"] = attachments
        mentions += literal_mentions(message.get("blocks", []))
        body = convert(message, logins, False, allow_attachment_summaries, channels, bool(references))
        summaries += bool(attachments)
        broadcasts += subtype == "thread_broadcast"
        content = {"status": "deleted"} if subtype == "tombstone" else {
            "status": "available", "body": {"format": "chatto-markdown", "text": body}, "asset_ids": references}
        reactions = collections.defaultdict(set)
        for reaction in original.get("reactions", []):
            for user in reaction.get("users", []):
                if user not in logins:
                    raise ApplyError("Unresolved reaction user.")
                reactions[reaction_unicode(reaction["name"], emojis)].add(user)
        event.update(type="message.snapshot", data={"message_id": key, "author_id": author,
            "thread_id": root, "reply_to": None, "content": content, "created_at": row["created_at"],
            "edited_at": row["edited_at"], "deleted_at": None,
            "reactions": [{"emoji": emoji, "user_ids": sorted(ids)} for emoji, ids in sorted(reactions.items())]})
        events.append(event)
        introduced.add(key)
    for room in plan["rooms"]:
        events.append({"id": "membership:" + room["source_id"], "type": "room.membership_snapshot",
                       "room_id": room["source_id"], "actor_id": None, "occurred_at": None,
                       "data": {"user_ids": sorted(set(room["members"]))}})
    omit("unsupported", "Structured user mentions are retained as literal names.", mentions)
    omit("unsupported", "Attachment text is retained; layout and embedded media are omitted.", summaries)
    omit("unsupported", "Thread broadcasts are retained in their thread without a channel echo.", broadcasts)
    workspaces = plan.get("slack_workspace_ids", [])
    if len(workspaces) > 1:
        raise ApplyError("Export identifies multiple Slack workspaces.")
    manifest = {"format": "chatto-interchange", "version": 1, "captured_at": None,
                "consistency": "unknown", "source": {"platform": "slack", "workspace_id": workspaces[0] if workspaces else "unknown"},
                "history": "partial", "scope": {"kind": "channels", "coverage": "selected"}, "omissions": omissions}
    records = {"manifest": manifest, "users": users, "rooms": rooms, "assets": assets, "events": events}
    # Identity includes exact source binding, enrichment, policy results and file
    # hashes. No wall-clock value makes an unchanged conversion a new import.
    manifest["export_id"] = str(uuid.uuid5(uuid.NAMESPACE_URL, fingerprint({"source": plan["archive_sha256"], "records": records})))
    validate_records(records)
    return records, copies


def validate_records(records):
    """Check the authoritative local schemas without network resolution or PII errors."""
    directory = Path(__file__).resolve().parents[1] / "schema/interchange/v1"
    schemas = {name: json.loads((directory / (name + ".schema.json")).read_text())
               for name in ("common", "manifest", "user", "room", "asset", "event")}
    registry = Registry().with_resources((s["$id"], Resource.from_contents(s)) for s in schemas.values())
    for name, schema in schemas.items():
        if name == "common":
            continue
        validator = Draft202012Validator(schema, registry=registry, format_checker=FormatChecker())
        rows = [records["manifest"]] if name == "manifest" else records[name + "s"]
        for index, row in enumerate(rows):
            if next(validator.iter_errors(row), None) is not None:
                raise ApplyError(f"Generated {name} record {index + 1} violates the interchange schema.")


def write_archive(output, records, copies):
    """Exclusively create a private output; write manifest last as completion marker.

    No system temporary directory, links, overwrite, or implicit output location.
    On interruption the incomplete directory stays for inspection; retry at a
    fresh output path. Download cache recovery is independent of this output.
    """
    output = Path(output)
    try:
        output.mkdir(mode=0o700)
    except FileExistsError:
        raise ApplyError("Output already exists; use a new output directory.") from None

    def private_file(path):
        return os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb")

    def sync_file(target):
        target.flush()
        os.fsync(target.fileno())

    def sync_directory(path):
        if os.name == "posix":
            descriptor = os.open(path, os.O_RDONLY)
            try:
                os.fsync(descriptor)
            finally:
                os.close(descriptor)

    for name in ("users", "rooms", "assets", "events"):
        with private_file(output / (name + ".jsonl")) as target:
            for record in records[name]:
                target.write((json.dumps(record, ensure_ascii=False, separators=(",", ":"), allow_nan=False) + "\n").encode())
            sync_file(target)
    if copies:
        (output / "files").mkdir(mode=0o700)
    for name, item in copies.items():
        digest, size = hashlib.sha256(), 0
        path = Path(item["path"])
        if path.is_symlink() or not path.is_file():
            raise ApplyError("Cached file is not a regular file.")
        with path.open("rb") as data, private_file(output / name) as target:
            while chunk := data.read(64 * 1024):
                size += len(chunk)
                if size > item["size"]:
                    raise ApplyError("Cached file changed during export.")
                digest.update(chunk)
                target.write(chunk)
            sync_file(target)
        if size != item["size"] or digest.hexdigest() != item["sha256"]:
            raise ApplyError("Cached file changed during export.")
    if copies:
        sync_directory(output / "files")
    sync_directory(output)
    with private_file(output / "manifest.json") as target:
        target.write((json.dumps(records["manifest"], ensure_ascii=False, indent=2, allow_nan=False) + "\n").encode())
        sync_file(target)
    sync_directory(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--output", required=True, type=Path)
    credentials = parser.add_mutually_exclusive_group()
    credentials.add_argument("--slack-token-file", type=Path)
    credentials.add_argument("--slack-token-stdin", action="store_true")
    parser.add_argument("--skip-files", action="store_true", help="do not download; requires --allow-missing-files")
    parser.add_argument("--allow-missing-files", action="store_true")
    parser.add_argument("--allow-attachment-summaries", action="store_true")
    parser.add_argument("--skip-canvases", action="store_true")
    parser.add_argument("--max-source-mib", type=int, default=256)
    parser.add_argument("--max-file-mib", type=int, default=25)
    parser.add_argument("--max-download-mib", type=int, default=1024)
    args = parser.parse_args()
    if min(args.max_source_mib, args.max_file_mib, args.max_download_mib) <= 0:
        parser.error("size limits must be positive")
    if args.skip_files and not args.allow_missing_files:
        parser.error("--skip-files requires --allow-missing-files")
    try:
        if args.output.exists() or args.output.is_symlink():
            raise ApplyError("Output already exists; use a new output directory.")
        # Only the reader's reserved folder can hold output inside a directory
        # source without becoming new source data on the next run.
        if args.archive.is_dir() and args.output.resolve().is_relative_to(args.archive.resolve()):
            if not args.output.resolve().is_relative_to(source.progress_directory(args.archive)):
                raise ApplyError("Output inside the source must be below .chatto-import/.")
        plan = source.read_export(args.archive, args.max_source_mib * 1024**2,
                                  include_slack_identity=True, include_interchange=True)
        if args.slack_token_file or args.slack_token_stdin:
            enrich_users(plan, SlackAPI(read_token(args.slack_token_file, sys.stdin)))
        # Reject content and reference gaps before potentially expensive downloads.
        build_archive(plan, allow_missing_files=True,
                      allow_attachment_summaries=args.allow_attachment_summaries, skip_canvases=args.skip_canvases)
        downloads = None
        if not args.skip_files and file_inventory(plan):
            downloads = FileImports(plan, FileCache(source.progress_directory(args.archive),
                args.max_file_mib * 1024**2, args.max_download_mib * 1024**2))
        records, copies = build_archive(plan, downloads, allow_missing_files=args.allow_missing_files,
            allow_attachment_summaries=args.allow_attachment_summaries, skip_canvases=args.skip_canvases)
        write_archive(args.output, records, copies)
        print(json.dumps({"users": len(records["users"]), "rooms": len(records["rooms"]),
                          "events": len(records["events"]), "assets": len(records["assets"]),
                          "file_bytes": sum(item["size"] for item in copies.values()),
                          "omissions": len(records["manifest"]["omissions"])}))
        return 0
    except (ApplyError, source.InvalidExport) as error:
        print(str(error), file=sys.stderr)
    except Exception:
        # Library exceptions can contain source names, URLs or credentials.
        print("Slack conversion failed. No source details are logged; incomplete output and download cache were retained.", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
