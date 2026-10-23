#!/usr/bin/env python3
"""Build a local Slack import plan. Never contact Slack or mutate Chatto.

The plan retains source content for a later operator-API writer. Reports contain
counts and issue codes only. Mapping files bind to the exact input archive.
"""

import argparse
import collections
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import zipfile


class InvalidExport(ValueError):
    """An export or mapping cannot be planned without guessing."""


def timestamp(value):
    """Convert Slack seconds.microseconds without a floating-point round trip."""
    if not isinstance(value, str) or not re.fullmatch(r"\d{1,12}\.\d{6}", value):
        raise InvalidExport("Invalid Slack message timestamp")
    seconds, fraction = value.split(".")
    try:
        date = datetime.datetime.fromtimestamp(int(seconds), datetime.timezone.utc)
    except (OverflowError, OSError, ValueError):
        raise InvalidExport("Slack timestamp is outside the supported range") from None
    return date.strftime("%Y-%m-%dT%H:%M:%S") + "." + fraction + "Z"


def records(value):
    """Accept Slack's empty-string placeholders, but reject other non-records."""
    if not isinstance(value, list):
        raise InvalidExport("Expected a JSON record array")
    if any(not isinstance(item, dict) and item != "" for item in value):
        raise InvalidExport("Invalid record in JSON array")
    return [item for item in value if isinstance(item, dict)]


def index_by_id(items):
    """Reject missing or duplicate source IDs instead of overwriting records."""
    result = {}
    for item in items:
        key = item.get("id")
        if not isinstance(key, str) or not key or key in result:
            raise InvalidExport("Missing or duplicate source ID")
        result[key] = item
    return result


def read_archive(path):
    """Read bounded JSON entries in place; no ZIP path is extracted."""
    with zipfile.ZipFile(path) as archive:
        entries = [entry for entry in archive.infolist() if not entry.is_dir()]
        names = [entry.filename for entry in entries]
        if len(names) != len(set(names)):
            raise InvalidExport("Duplicate ZIP entry")
        if sum(entry.file_size for entry in entries) > 256 * 1024 * 1024:
            raise InvalidExport("Export exceeds the 256 MiB inspection limit")
        data = {}
        for entry in entries:
            if entry.filename.endswith(".json"):
                data[entry.filename] = json.loads(archive.read(entry))
        if "users.json" not in data or "channels.json" not in data:
            raise InvalidExport("Export must contain users.json and channels.json")
        return data, names


def make_plan(path, mapping=None):
    """Return a source plan, reusable mapping, and content-free summary.

    Null destination IDs mean unresolved, not permission to create an account.
    The plan is not an executable API request file. In particular, source rich
    text remains unchanged until a writer implements and verifies conversion.
    """
    with open(path, "rb") as source:
        digest = hashlib.file_digest(source, "sha256").hexdigest()
    data, filenames = read_archive(path)
    users = index_by_id(records(data["users.json"]))
    channels = index_by_id(records(data["channels.json"]))
    names = [channel.get("name") for channel in channels.values()]
    if any(not isinstance(name, str) or not name or "/" in name for name in names):
        raise InvalidExport("Invalid channel directory name")
    if len(names) != len(set(names)):
        raise InvalidExport("Ambiguous channel directory name")
    expected = {"version": 1, "archive_sha256": digest,
                "users": {key: None for key in users},
                "rooms": {key: None for key in channels}}
    if mapping is None:
        mapping = expected
    else:
        if not isinstance(mapping, dict):
            raise InvalidExport("Mapping must be an object")
        if mapping.get("version") != 1 or mapping.get("archive_sha256") != digest:
            raise InvalidExport("Mapping does not match this archive")
        for group in ("users", "rooms"):
            values = mapping.get(group)
            if not isinstance(values, dict) or set(values) != set(expected[group]):
                raise InvalidExport("Mapping source IDs do not match this archive")
            if any(value is not None and (not isinstance(value, str) or not value.strip())
                   for value in values.values()):
                raise InvalidExport("Destination IDs must be nonempty strings or null")
            targets = [value for value in values.values() if value is not None]
            if len(targets) != len(set(targets)):
                raise InvalidExport("Mapping merges multiple source identities; resolve explicitly")

    issues = collections.Counter()
    planned_users = []
    for key, user in users.items():
        profile = user.get("profile") or {}
        planned_users.append({
            "source_id": key, "target_id": mapping["users"][key],
            "proposed_login": user.get("name"),
            "proposed_display_name": profile.get("display_name") or profile.get("real_name") or user.get("name"),
            "deactivated": bool(user.get("deleted")),
            "bot": bool(user.get("is_bot") or user.get("is_app_user")),
            "has_avatar": bool(profile.get("image_original")),
        })
        if user.get("deleted"):
            issues["deactivated_user_policy"] += 1
        if user.get("is_bot") or user.get("is_app_user"):
            issues["bot_identity_policy"] += 1
        if profile.get("image_original"):
            issues["operator_avatar_import_missing"] += 1

    planned_rooms, messages = [], []
    consumed = set()
    for key, channel in channels.items():
        members = channel.get("members", [])
        if not isinstance(members, list) or any(member not in users for member in members):
            raise InvalidExport("Unresolved channel member")
        planned_rooms.append({
            "source_id": key, "target_id": mapping["rooms"][key],
            "name": channel["name"], "topic": channel.get("topic"),
            "purpose": channel.get("purpose"), "members": members,
            "archived": bool(channel.get("is_archived")),
            "general": bool(channel.get("is_general")),
        })
        if channel.get("is_archived"):
            issues["operator_archive_room_missing"] += 1
        prefix = channel["name"] + "/"
        for filename in sorted(data):
            if not filename.startswith(prefix):
                continue
            consumed.add(filename)
            for message in records(data[filename]):
                ts = message.get("ts")
                created_at = timestamp(ts)
                author = message.get("user")
                if author not in users:
                    issues["unresolved_message_author"] += 1
                body = message.get("text", "")
                if not isinstance(body, str):
                    raise InvalidExport("Message text must be a string")
                root = message.get("thread_ts")
                if root is not None:
                    timestamp(root)
                edited = message.get("edited")
                edited_at = timestamp(edited["ts"]) if edited else None
                if edited and tuple(map(int, edited["ts"].split("."))) < tuple(map(int, ts.split("."))):
                    raise InvalidExport("Message edit precedes creation")
                reactions = message.get("reactions", [])
                for reaction in reactions:
                    reactors = reaction.get("users", [])
                    if any(user not in users for user in reactors):
                        issues["unresolved_reaction_user"] += 1
                    if reaction.get("count") != len(set(reactors)):
                        issues["incomplete_reaction_users"] += 1
                    issues["reaction_import_required"] += len(reactors)
                for file in message.get("files", []):
                    if file.get("mode") == "hidden_by_limit":
                        issues["file_hidden_by_limit"] += 1
                    else:
                        issues["file_download_or_local_mapping_required"] += 1
                if root and root != ts:
                    issues["thread_import_required"] += 1
                if message.get("subtype"):
                    issues["message_subtype_review"] += 1
                if message.get("attachments"):
                    issues["slack_attachment_conversion_required"] += 1
                if re.search(r"<[@#!]|<https?://", body) or message.get("blocks"):
                    issues["content_conversion_required"] += 1
                messages.append({
                    "source_key": key + ":" + ts, "source_channel_id": key,
                    "target_room_id": mapping["rooms"][key],
                    "source_author_id": author, "target_author_id": mapping["users"].get(author),
                    "created_at": created_at, "edited_at": edited_at,
                    "thread_root_key": key + ":" + root if root else None,
                    "source": message,
                })

    # Import order must follow source time, even when ZIP entries are unordered.
    messages.sort(key=lambda item: (item["source_channel_id"], tuple(map(int, item["source"]["ts"].split(".")))))
    message_index = {item["source_key"]: item for item in messages}
    if len(message_index) != len(messages):
        raise InvalidExport("Duplicate message source key")
    reply_counts = collections.Counter(item["thread_root_key"] for item in messages
                                      if item["thread_root_key"] != item["source_key"])
    for item in messages:
        root = item["thread_root_key"]
        if root and root not in message_index:
            issues["missing_thread_root"] += 1
        declared = item["source"].get("reply_count")
        if declared is not None:
            actual = reply_counts[item["source_key"]]
            if declared != actual:
                issues["incomplete_thread_replies"] += 1
    if messages:
        issues["uncertain_write_recovery_required"] = 1
    canvases = records(data.get("canvases.json", []))
    if canvases:
        issues["canvas_content_not_imported"] = len(canvases)
    known = {"users.json", "channels.json", "canvases.json", ".slack-manifest.json",
             "integration_logs.json", "lists.json", "huddle_transcripts.json", "file_conversations.json"}
    unhandled = sorted(set(filenames) - consumed - known)
    if unhandled:
        issues["unhandled_archive_entries"] = len(unhandled)
    for name in ("lists.json", "huddle_transcripts.json", "file_conversations.json"):
        count = len(records(data.get(name, [])))
        if count:
            issues["unsupported_" + name.removesuffix(".json")] = count
    summary = {
        "users": len(users), "rooms": len(channels), "messages": len(messages),
        "thread_roots": sum(item["thread_root_key"] == item["source_key"] for item in messages),
        "thread_replies": issues["thread_import_required"],
        "reactions": issues["reaction_import_required"],
        "files": sum(len(item["source"].get("files", [])) for item in messages),
        "canvases": len(canvases),
        "unmapped_users": sum(value is None for value in mapping["users"].values()),
        "unmapped_rooms": sum(value is None for value in mapping["rooms"].values()),
        "issues": dict(sorted(issues.items())),
    }
    plan = {"version": 1, "archive_sha256": digest, "mode": "plan-only",
            "summary": summary, "users": planned_users, "rooms": planned_rooms,
            "messages": messages, "canvases": canvases, "unhandled_entries": unhandled,
            "ignored_integration_logs": len(records(data.get("integration_logs.json", [])))}
    return plan, mapping, summary


def write_private(path, value):
    """Create a new owner-only artifact. Never overwrite mappings or follow links."""
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as output:
        json.dump(value, output, ensure_ascii=False, indent=2)
        output.write("\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--mapping", type=Path, help="existing mapping from a previous plan")
    parser.add_argument("--output-dir", type=Path, required=True, help="new private directory for plan, mapping, and report")
    args = parser.parse_args()
    try:
        mapping = json.loads(args.mapping.read_text()) if args.mapping else None
        plan, mapping, summary = make_plan(args.archive, mapping)
        args.output_dir.mkdir(mode=0o700, parents=True, exist_ok=False)
        for name, value in (("plan.json", plan), ("mapping.json", mapping), ("report.json", summary)):
            write_private(args.output_dir / name, value)
        print(json.dumps(summary, indent=2))
    except InvalidExport as error:
        print(str(error), file=sys.stderr)
        return 1
    except (OSError, ValueError, KeyError, TypeError, AttributeError, RuntimeError, zipfile.BadZipFile):
        # Parser and filesystem exceptions can contain private source data.
        print("Cannot build plan. Check input structure, paths, and a new output directory.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
