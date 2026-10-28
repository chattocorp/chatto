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
import re
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
    """Keep unknown reaction names; retain Unicode output for known Slack emoji."""
    if not isinstance(name, str):
        raise ApplyError("Invalid reaction name.")
    base, separator, tone = name.partition("::skin-tone-")
    value = emojis.get({"hugging_face": "hugs"}.get(base, base))
    if not value:
        if re.fullmatch(r"[A-Za-z0-9_+-]{1,128}", name):
            return name
        raise ApplyError("Invalid or unsupported reaction emoji.")
    if separator:
        if tone not in {"2", "3", "4", "5", "6"} or "\u200d" in value:
            raise ApplyError("Unsupported reaction skin-tone sequence.")
        value = value.rstrip("\ufe0f") + chr(0x1F3FB + int(tone) - 2)
    return value


def complete_bot_catalog(plan, users):
    """Resolve bot senders from stable source IDs, never from display names.

    Legacy integrations can have bot_id/username but no user row. Slack's
    USLACKBOT identity can also be referenced without appearing in users.json.
    These historical identities carry no invented emails or credentials.
    """
    by_id = {u["id"]: u for u in users}
    bot_authors = {}

    def bind(bot_id, user_id):
        if not isinstance(bot_id, str) or not bot_id:
            raise ApplyError("Invalid bot identity.")
        previous = bot_authors.get(bot_id)
        if previous is not None and previous != user_id:
            raise ApplyError("Conflicting bot identity references.")
        bot_authors[bot_id] = user_id

    if any(row["source_author_id"] == "USLACKBOT" for row in plan["messages"]) and "USLACKBOT" not in by_id:
        user = {"id": "USLACKBOT", "username": "slackbot", "display_name": "Slackbot",
                "kind": "bot", "status": "active", "emails": []}
        users.append(user)
        by_id[user["id"]] = user
    for user in plan["users"]:
        if user.get("source_bot_id") and by_id[user["source_id"]]["kind"] == "bot":
            bind(user["source_bot_id"], user["source_id"])
    for row in plan["messages"]:
        bot_id, author = row["source"].get("bot_id"), row["source_author_id"]
        if bot_id and author in by_id and by_id[author]["kind"] == "bot":
            bind(bot_id, author)
    for row in plan["messages"]:
        message = row["source"]
        bot_id = message.get("bot_id")
        if row["source_author_id"] or not bot_id:
            continue
        if not isinstance(bot_id, str):
            raise ApplyError("Invalid bot identity.")
        if bot_id in bot_authors:
            continue
        name = message.get("username") or (message.get("bot_profile") or {}).get("name")
        if not isinstance(name, str) or not name:
            raise ApplyError("Bot identity lacks a source display name.")
        identity = "slack-bot:" + bot_id
        if identity in by_id:
            raise ApplyError("Conflicting bot catalog identity.")
        user = {"id": identity, "username": None, "display_name": name,
                "kind": "bot", "status": "active", "emails": []}
        users.append(user)
        by_id[identity] = user
        bind(bot_id, identity)
    return bot_authors


def prepare_message(original):
    """Share content normalization between diagnostics and archive conversion.

    File references become archive asset references; structured mentions remain
    literal text. Work on a copy so repeated scans never change source records.
    """
    message = copy.deepcopy(original)
    references = list(dict.fromkeys(f["id"] for f in file_references(message)))
    message["files"] = []
    attachments = []
    for attachment in message.get("attachments", []):
        had_files = bool(attachment.get("files"))
        attachment["files"] = []
        if not (had_files and set(attachment) <= {"files", "id"}):
            attachments.append(attachment)
    if "attachments" in message:
        message["attachments"] = attachments
    mentions = literal_mentions(message.get("blocks", []))
    return message, references, mentions, bool(attachments)


def conversion_report(plan, *, allow_attachment_summaries=False, skip_canvases=False,
                      allow_missing_files=False):
    """Scan independent source records without downloads or destination writes.

    References are one-based positions in the reader's ordered message list,
    not bodies, names, URLs, or credentials. Later archive validation remains
    authoritative: this diagnostic scan does not prove importability.
    """
    report = {"version": 1, "status": "scanned", "archive_written": False,
              "file_checks": "not_run", "blockers": [], "omissions": [],
              "transformations": [], "reference_order": "room ID, then Slack timestamp",
              "source_sha256": plan["archive_sha256"]}
    groups = {}

    def add(section, code, count=1, position=None):
        key = section, code
        if key not in groups:
            groups[key] = {"code": code, "count": 0, "source_records": []}
            report[section].append(groups[key])
        item = groups[key]
        item["count"] += count
        if position is not None and (not item["source_records"] or item["source_records"][-1] != position):
            item["source_records"].append(position)

    issues = plan["summary"]["issues"]
    for code in ("unhandled_archive_entries", "unsupported_lists", "unsupported_huddle_transcripts",
                 "unsupported_file_conversations", "missing_thread_root", "invalid_thread_root"):
        if issues.get(code):
            add("blockers", code, issues[code])
    add("omissions", "incomplete_source_history")
    for code, count in (("avatars", sum(u["has_avatar"] for u in plan["users"])),
                        ("integration_logs", plan["ignored_integration_logs"]),
                        ("incomplete_thread_replies", issues.get("incomplete_thread_replies", 0)),
                        ("incomplete_reaction_users", issues.get("incomplete_reaction_users", 0))):
        if count:
            add("omissions", code, count)
    if plan["canvases"]:
        add("omissions" if skip_canvases else "blockers", "canvases", len(plan["canvases"]))
    users = [{"id": u["source_id"], "kind": "bot" if u["bot"] or u["source_id"] == "USLACKBOT" else "human"}
             for u in plan["users"]]
    try:
        bots = complete_bot_catalog(plan, users)
    except ApplyError:
        add("blockers", "bot_identity_conflict_or_missing_metadata")
        bots = {}
    logins = {u["source_id"]: u["proposed_login"] or u["source_id"] for u in plan["users"]}
    logins.update({u["id"]: u.get("display_name", u["id"]) for u in users if u["id"] not in logins})
    channels = {r["source_id"]: r["name"] for r in plan["rooms"]}
    emojis = json.loads(Path(__file__).with_name("emoji-map.json").read_text())
    unknown_names = set()
    introduced, membership = set(), {}
    for position, row in enumerate(plan["messages"], 1):
        original = row["source"]
        subtype = original.get("subtype")
        author = row["source_author_id"] or bots.get(original.get("bot_id"))
        if author not in logins and (subtype not in CHANNEL_NOTICES or subtype in {"channel_join", "channel_leave"}):
            add("blockers", "unresolved_author", position=position)
        for reaction in original.get("reactions", []):
            try:
                reaction_unicode(reaction["name"], emojis)
            except ApplyError:
                add("blockers", "unsupported_reaction_emoji", position=position)
                unknown_names.add(reaction["name"])
            if any(user not in logins for user in reaction.get("users", [])):
                add("blockers", "unresolved_reactor", position=position)
        if subtype in CHANNEL_NOTICES:
            if original.get("blocks") or file_references(original) or original.get("attachments") or original.get("reactions"):
                add("blockers", "channel_event_content", position=position)
            if subtype in {"channel_join", "channel_leave"}:
                member = row["source_channel_id"], author
                present = subtype == "channel_join"
                if member in membership and membership[member] == present:
                    add("blockers", "repeated_membership_transition", position=position)
                membership[member] = present
                if original.get("inviter") and original["inviter"] not in logins:
                    add("blockers", "unresolved_membership_actor", position=position)
            field = {"channel_name": "name", "channel_topic": "topic", "channel_purpose": "purpose"}.get(subtype)
            if field and (not isinstance(original.get(field), str) or field == "name" and not original[field]):
                add("blockers", "channel_metadata_missing", position=position)
            continue
        root = row["thread_root_key"]
        if root and root != row["source_key"] and root not in introduced:
            add("blockers", "thread_root_not_before_reply", position=position)
        introduced.add(row["source_key"])
        message, references, mentions, attachments = prepare_message(original)
        if mentions:
            add("transformations", "literal_user_mentions", mentions, position)
        if attachments:
            add("transformations" if allow_attachment_summaries else "blockers",
                "attachment_summaries", position=position)
        if original.get("subtype") == "thread_broadcast":
            add("transformations", "thread_broadcast_without_channel_echo", position=position)
        if original.get("subtype") == "tombstone" and references:
            add("blockers", "tombstone_content", position=position)
        try:
            convert(message, logins, False, allow_attachment_summaries, channels, bool(references))
        except ApplyError:
            add("blockers", "unsupported_message_content", position=position)
    report["unsupported_emoji_names"] = len(unknown_names)
    try:
        inventory = file_inventory(plan)
        report["source_assets"] = len(inventory)
        for item in inventory.values():
            mode = item.get("mode")
            reason = None
            if mode == "hidden_by_limit":
                reason = "files_hidden_by_limit"
            elif mode == "tombstone":
                reason = "files_deleted"
            elif item.get("is_external") or mode == "external" or item.get("filetype") == "remote":
                reason = "external_files"
            if reason:
                add("omissions" if allow_missing_files else "blockers", reason)
    except ApplyError:
        add("blockers", "conflicting_file_metadata")
    return report


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
    bot_authors = complete_bot_catalog(plan, users)
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
    custom_reaction_names = set()
    mentions = summaries = broadcasts = 0
    for row in plan["messages"]:
        original = row["source"]
        subtype, key, room = original.get("subtype"), row["source_key"], row["source_channel_id"]
        event = {"id": "event:" + key, "room_id": room, "actor_id": None,
                 "occurred_at": None}
        author = row["source_author_id"]
        if not author and original.get("bot_id"):
            author = bot_authors.get(original["bot_id"])
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
        message, references, mention_count, has_attachments = prepare_message(original)
        if subtype == "tombstone" and references:
            raise ApplyError("Tombstone contains unsupported content.")
        mentions += mention_count
        body = convert(message, logins, False, allow_attachment_summaries, channels, bool(references))
        summaries += has_attachments
        broadcasts += subtype == "thread_broadcast"
        content = {"status": "deleted"} if subtype == "tombstone" else {
            "status": "available", "body": {"format": "chatto-markdown", "text": body}, "asset_ids": references}
        reactions = collections.defaultdict(set)
        for reaction in original.get("reactions", []):
            for user in reaction.get("users", []):
                if user not in logins:
                    raise ApplyError("Unresolved reaction user.")
                emoji = reaction_unicode(reaction["name"], emojis)
                reactions[emoji].add(user)
                # Unresolved names retain their identity but have no image in the archive.
                if emoji == reaction["name"]:
                    custom_reaction_names.add(emoji)
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
    omit("unsupported", "Custom reaction names are retained; their emoji images are not included.", len(custom_reaction_names))
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
    parser.add_argument("--report", type=Path, help="new report file; default: OUTPUT.conversion-report.json")
    parser.add_argument("--report-only", action="store_true", help="scan source gaps without downloads or archive creation")
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
    report = {"version": 1, "status": "failed", "archive_written": False,
              "file_checks": "not_run", "blockers": [], "omissions": [], "transformations": []}
    report_file = None
    stage = "setup"
    try:
        if args.output.exists() or args.output.is_symlink():
            raise ApplyError("Output already exists; use a new output directory.")
        # Only the reader's reserved folder can hold output inside a directory
        # source without becoming new source data on the next run.
        if args.archive.is_dir() and args.output.resolve().is_relative_to(args.archive.resolve()):
            if not args.output.resolve().is_relative_to(source.progress_directory(args.archive)):
                raise ApplyError("Output inside the source must be below .chatto-import/.")
        report_path = args.report or args.output.with_name(args.output.name + ".conversion-report.json")
        if report_path.resolve().is_relative_to(args.output.resolve()):
            raise ApplyError("Report must be outside the archive output directory.")
        if args.archive.is_dir() and report_path.resolve().is_relative_to(args.archive.resolve()):
            if not report_path.resolve().is_relative_to(source.progress_directory(args.archive)):
                raise ApplyError("Report inside the source must be below .chatto-import/.")
        report_file = os.fdopen(os.open(report_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "w")
        stage = "source_read"
        plan = source.read_export(args.archive, args.max_source_mib * 1024**2,
                                  include_slack_identity=True, include_interchange=True)
        stage = "account_enrichment"
        if args.slack_token_file or args.slack_token_stdin:
            enrich_users(plan, SlackAPI(read_token(args.slack_token_file, sys.stdin)))
        stage = "source_scan"
        report = conversion_report(plan, allow_attachment_summaries=args.allow_attachment_summaries,
                                   skip_canvases=args.skip_canvases, allow_missing_files=args.allow_missing_files)
        if report["blockers"]:
            report["status"] = "blocked"
            print("Conversion blocked; see conversion report.", file=sys.stderr)
            return 1
        if args.report_only:
            return 0
        # Reject content and reference gaps before potentially expensive downloads.
        stage = "archive_validation"
        build_archive(plan, allow_missing_files=True,
                      allow_attachment_summaries=args.allow_attachment_summaries, skip_canvases=args.skip_canvases)
        downloads = None
        stage = "file_downloads"
        if not args.skip_files and file_inventory(plan):
            downloads = FileImports(plan, FileCache(source.progress_directory(args.archive),
                args.max_file_mib * 1024**2, args.max_download_mib * 1024**2))
        report["file_checks"] = "skipped" if args.skip_files else "complete"
        if downloads:
            report["file_failures"] = dict(collections.Counter(downloads.failures.values()))
        stage = "archive_validation"
        records, copies = build_archive(plan, downloads, allow_missing_files=args.allow_missing_files,
            allow_attachment_summaries=args.allow_attachment_summaries, skip_canvases=args.skip_canvases)
        report["archive_omissions"] = records["manifest"]["omissions"]
        stage = "archive_write"
        write_archive(args.output, records, copies)
        report.update(status="complete", archive_written=True)
        print(json.dumps({"users": len(records["users"]), "rooms": len(records["rooms"]),
                          "events": len(records["events"]), "assets": len(records["assets"]),
                          "file_bytes": sum(item["size"] for item in copies.values()),
                          "omissions": len(records["manifest"]["omissions"])}))
        return 0
    except (ApplyError, source.InvalidExport) as error:
        report["status"] = "failed"
        report["blockers"].append({"code": stage + "_failed", "count": 1, "source_records": []})
        print(str(error), file=sys.stderr)
    except Exception:
        report["status"] = "failed"
        report["blockers"].append({"code": stage + "_failed", "count": 1, "source_records": []})
        # Library exceptions can contain source names, URLs or credentials.
        print("Slack conversion failed. No source details are logged; incomplete output and download cache were retained.", file=sys.stderr)
    finally:
        if report_file is not None:
            try:
                with report_file:
                    json.dump(report, report_file, indent=2, allow_nan=False)
                    report_file.write("\n")
                    report_file.flush()
                    os.fsync(report_file.fileno())
            except OSError:
                print("Could not finish conversion report.", file=sys.stderr)
                return 1
    return 1


if __name__ == "__main__":
    sys.exit(main())
