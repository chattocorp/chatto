#!/usr/bin/env python3
"""Import a Slack ZIP into a blank Chatto instance through its operator socket."""

import argparse
import collections
import datetime
import hashlib
import http.client
import json
from pathlib import Path
import re
import sys
import zipfile

from slack_apply import ApplyError, Journal, Operator, apply, fingerprint


class InvalidExport(ValueError):
    """An export cannot be read without guessing."""


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


def read_export(path):
    """Parse and validate source records in memory before any destination writes."""
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
    issues = collections.Counter()
    planned_users = []
    for key, user in users.items():
        profile = user.get("profile") or {}
        planned_users.append({
            "source_id": key,
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
            "source_id": key,
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
                if message.get("type") != "message":
                    raise InvalidExport("Unsupported channel record type")
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
                    "source_author_id": author,
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
        elif root and message_index[root]["thread_root_key"] not in (None, root):
            issues["invalid_thread_root"] += 1
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
        "issues": dict(sorted(issues.items())),
    }
    plan = {"version": 1, "archive_sha256": digest,
            "summary": summary, "users": planned_users, "rooms": planned_rooms,
            "messages": messages, "canvases": canvases, "unhandled_entries": unhandled,
            "ignored_integration_logs": len(records(data.get("integration_logs.json", [])))}
    return plan


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--operator-socket", type=Path, required=True)
    parser.add_argument("--allow-missing-files", action="store_true")
    parser.add_argument("--skip-canvases", action="store_true")
    parser.add_argument("--reconcile", type=Path, help="reviewed pending-write resolution JSON; exits without importing")
    args = parser.parse_args()
    try:
        plan = read_export(args.archive)
        api = Operator(args.operator_socket)
        policy = {"allow_missing_files": args.allow_missing_files, "skip_canvases": args.skip_canvases}
        binding = {"source": fingerprint(plan), "policy": policy, "socket": api.path}
        journal = Journal(args.archive.resolve().with_name(args.archive.name + ".chatto-import"), binding)
        try:
            if args.reconcile:
                journal.reconcile(json.loads(args.reconcile.read_text()))
                print("Reconciliation recorded. Repeat the import command to resume.")
            else:
                print(json.dumps(apply(plan, api, journal, **policy), indent=2))
        finally:
            journal.close()
    except (InvalidExport, ApplyError) as error:
        print(str(error), file=sys.stderr)
        return 1
    except (OSError, ValueError, KeyError, TypeError, AttributeError, RuntimeError,
            http.client.HTTPException, zipfile.BadZipFile):
        # Parser and filesystem exceptions can contain private source data.
        print("Import stopped. Progress was retained; repeat the command to resume.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
