#!/usr/bin/env python3
"""Inspect or import a Slack ZIP or directory without copying source files."""

import argparse
import collections
import datetime
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import sys
import unicodedata
import zipfile

from slack_apply import ApplyError, SQLiteJournal, Operator, apply, convert, fingerprint, save
from slack_api import SlackAPI, enrich_users, read_token
from slack_files import FileCache, FileImports, file_inventory


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


def read_archive(path, max_bytes=256 * 1024 * 1024):
    """Read bounded JSON entries in place; no ZIP path is extracted."""
    with zipfile.ZipFile(path) as archive:
        entries = [entry for entry in archive.infolist() if not entry.is_dir()]
        names = [unicodedata.normalize("NFC", entry.filename) for entry in entries]
        if len(names) != len(set(names)):
            raise InvalidExport("Duplicate ZIP entry")
        if sum(entry.file_size for entry in entries) > max_bytes:
            raise InvalidExport("Export exceeds the configured inspection limit")
        data = {}
        for entry, name in zip(entries, names):
            if entry.filename.endswith(".json"):
                data[name] = json.loads(archive.read(entry))
        if "users.json" not in data or "channels.json" not in data:
            raise InvalidExport("Export must contain users.json and channels.json")
        return data, names


def read_directory(path, max_bytes):
    """Read regular files in place; reserve .chatto-import for local progress.

    Hash relative names and file bytes in stable order to bind resume to the
    source. Reject symlinks and special files instead of reading outside it.
    """
    files = []
    total = 0
    def unreadable_directory(error):
        raise InvalidExport("Cannot read an export directory") from None

    for directory, dirs, names in os.walk(path, followlinks=False, onerror=unreadable_directory):
        if Path(directory) == path and ".chatto-import" in dirs:
            dirs.remove(".chatto-import")
        for name in dirs + names:
            entry = Path(directory) / name
            if entry.is_symlink():
                raise InvalidExport("Export contains a symbolic link")
        for name in names:
            entry = Path(directory) / name
            if not entry.is_file():
                raise InvalidExport("Export contains a non-regular file")
            total += entry.stat().st_size
            if total > max_bytes:
                raise InvalidExport("Export exceeds the configured inspection limit")
            files.append(entry)
    digest = hashlib.sha256(b"chatto-slack-directory-v1\0")
    data, names = {}, []
    total = 0
    for entry in sorted(files):
        name = entry.relative_to(path).as_posix()
        with entry.open("rb") as source:
            content = source.read(max_bytes - total + 1)
        total += len(content)
        if total > max_bytes:
            raise InvalidExport("Export exceeds the configured inspection limit")
        digest.update(json.dumps([name, len(content)]).encode() + b"\0")
        digest.update(content)
        name = unicodedata.normalize("NFC", name)
        if name in data:
            raise InvalidExport("Duplicate normalized export path")
        names.append(name)
        if name.endswith(".json"):
            data[name] = json.loads(content)
    if "users.json" not in data or "channels.json" not in data:
        raise InvalidExport("Export must contain users.json and channels.json")
    return data, names, digest.hexdigest()


def read_export(path, max_bytes=256 * 1024 * 1024, include_slack_identity=False, include_interchange=False):
    """Parse and validate source records in memory before any destination writes."""
    if path.is_dir():
        data, filenames, digest = read_directory(path.resolve(), max_bytes)
    else:
        with open(path, "rb") as source:
            digest = hashlib.file_digest(source, "sha256").hexdigest()
        data, filenames = read_archive(path, max_bytes)
    users = index_by_id(records(data["users.json"]))
    channels = index_by_id(records(data["channels.json"]))
    names = [channel.get("name") for channel in channels.values()]
    if any(not isinstance(name, str) or not name or "/" in name for name in names):
        raise InvalidExport("Invalid channel directory name")
    if len(names) != len({unicodedata.normalize("NFC", name) for name in names}):
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
        if include_slack_identity:
            planned_users[-1]["source_email"] = profile.get("email")
        if include_interchange:
            planned_users[-1]["source_bot_id"] = profile.get("bot_id")
            from slack_avatars import avatar_urls
            planned_users[-1]["avatar_urls"] = avatar_urls(profile)
            planned_users[-1]["avatar_url"] = next(iter(planned_users[-1]["avatar_urls"]), None)
            planned_users[-1]["has_avatar"] = bool(planned_users[-1]["avatar_url"])
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
        if include_interchange:
            planned_rooms[-1]["private"] = bool(channel.get("is_private"))
        if channel.get("is_archived"):
            issues["operator_archive_room_missing"] += 1
        prefix = unicodedata.normalize("NFC", channel["name"]) + "/"
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
                files = message.get("files", []) + [file for attachment in message.get("attachments", [])
                                                    for file in attachment.get("files", [])]
                for file in files:
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
    if include_interchange:
        unhandled = [name for name in unhandled if Path(name).name != ".DS_Store"]
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
        "files": issues["file_hidden_by_limit"] + issues["file_download_or_local_mapping_required"],
        "canvases": len(canvases),
        "issues": dict(sorted(issues.items())),
    }
    plan = {"version": 1, "archive_sha256": digest,
            "summary": summary, "users": planned_users, "rooms": planned_rooms,
            "messages": messages, "canvases": canvases, "unhandled_entries": unhandled,
            "ignored_integration_logs": len(records(data.get("integration_logs.json", [])))}
    if include_slack_identity:
        plan["slack_workspace_ids"] = sorted({u["team_id"] for u in users.values()
                                             if isinstance(u.get("team_id"), str) and u["team_id"]})
    return plan


def preflight(plan, file_importer=None, channel_prefix=""):
    """Return counts only, with no destination access or source identifiers.

    Conversion assumes file placeholders so content gaps remain visible.
    Each failed message contributes its first conversion error only. This
    check does not prove destination readiness or approve source omissions.
    """
    logins = {user["source_id"]: "user" + str(i) for i, user in enumerate(plan["users"])}
    failures = collections.Counter()
    channels = {room["source_id"]: channel_prefix + room["name"] for room in plan["rooms"]}
    for message in plan["messages"]:
        try:
            source, files = file_importer.prepare_message(message["source"], True) if file_importer else (message["source"], [])
            convert(source, logins, allow_missing_files=True, allow_attachment_summaries=True, channels=channels,
                    has_attachments=bool(files))
        except ApplyError as error:
            failures[str(error)] += 1
    return {"summary": plan["summary"],
            "conversion_assumes_missing_file_placeholders": True,
            "conversion_assumes_attachment_summaries": True,
            "convertible_messages": len(plan["messages"]) - sum(failures.values()),
            "conversion_failures": dict(sorted(failures.items()))}


def progress_directory(source):
    """Keep directory-source progress inside the export, ZIP progress beside it."""
    source = source.resolve()
    directory = source / ".chatto-import" if source.is_dir() else source.with_name(source.name + ".chatto-import")
    if directory.is_symlink():
        raise InvalidExport("Progress directory must not be a symbolic link")
    return directory


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--operator-socket", type=Path)
    credentials = parser.add_mutually_exclusive_group()
    credentials.add_argument("--slack-token-file", type=Path,
                             help="read Slack API token from a private file; import matched account emails")
    credentials.add_argument("--slack-token-stdin", action="store_true",
                             help="read Slack API token from stdin; import matched account emails")
    parser.add_argument("--preflight", action="store_true", help="print counts and conversion gaps without contacting Chatto")
    parser.add_argument("--save-report", action="store_true", help="save preflight counts in the local progress directory")
    parser.add_argument("--max-source-mib", type=int, default=256, help="maximum uncompressed source size (default: 256)")
    parser.add_argument("--allow-missing-files", action="store_true")
    parser.add_argument("--channel-prefix", default="", help="optional prefix for imported channel names (default: none)")
    parser.add_argument("--download-files", action="store_true",
                        help="download exported file URLs into the source progress directory, including during preflight")
    parser.add_argument("--max-file-mib", type=int, default=25, help="maximum downloaded file size (default: 25 MiB)")
    parser.add_argument("--max-download-mib", type=int, default=1024,
                        help="maximum downloaded bytes per run, including retries (default: 1024 MiB)")
    parser.add_argument("--allow-attachment-summaries", action="store_true",
                        help="preserve attachment text summaries without Slack layout or embedded media")
    parser.add_argument("--skip-canvases", action="store_true")
    parser.add_argument("--reconcile", type=Path, help="reviewed pending-write resolution JSON; exits without importing")
    args = parser.parse_args()
    if args.max_source_mib <= 0:
        parser.error("--max-source-mib must be positive")
    if args.max_file_mib <= 0 or args.max_download_mib <= 0:
        parser.error("file and download limits must be positive")
    if args.save_report and not args.preflight:
        parser.error("--save-report requires --preflight")
    if args.preflight and (args.operator_socket or args.reconcile):
        parser.error("--preflight cannot use an operator socket or reconciliation")
    if not args.preflight and not args.operator_socket:
        parser.error("import requires --operator-socket")
    try:
        online = bool(args.slack_token_file or args.slack_token_stdin)
        plan = read_export(args.archive, args.max_source_mib * 1024 * 1024, include_slack_identity=online)
        identity_report = None
        if online:
            token = read_token(args.slack_token_file, sys.stdin)
            identity_report = enrich_users(plan, SlackAPI(token))
            del token
        file_importer = None
        if args.download_files:
            file_importer = FileImports(plan, FileCache(progress_directory(args.archive),
                args.max_file_mib * 1024**2, args.max_download_mib * 1024**2))
        if args.preflight:
            report = preflight(plan, file_importer, args.channel_prefix)
            report["unique_file_ids"] = len(file_inventory(plan))
            if file_importer:
                report["file_downloads"] = file_importer.report()
            if identity_report is not None:
                report["slack_api"] = identity_report
            if args.save_report:
                directory = progress_directory(args.archive)
                directory.mkdir(mode=0o700, exist_ok=True)
                save(directory / "preflight.json", report)
            print(json.dumps(report, indent=2))
            return 0
        api = Operator(args.operator_socket)
        policy = {"allow_missing_files": args.allow_missing_files, "skip_canvases": args.skip_canvases,
                  "channel_prefix": args.channel_prefix}
        if args.allow_attachment_summaries:
            policy["allow_attachment_summaries"] = True
        binding = {"source": fingerprint(plan), "policy": policy, "socket": api.path}
        if file_importer:
            binding["files"] = file_importer.binding()
        journal = SQLiteJournal(progress_directory(args.archive), binding)
        try:
            if args.reconcile:
                journal.reconcile(json.loads(args.reconcile.read_text()))
                print("Reconciliation recorded. Repeat the import command to resume.")
            else:
                print(json.dumps(apply(plan, api, journal, file_importer=file_importer, **policy), indent=2))
        finally:
            journal.close()
            api.close()
    except (InvalidExport, ApplyError) as error:
        print(str(error), file=sys.stderr)
        return 1
    except (OSError, ValueError, KeyError, TypeError, AttributeError, RuntimeError,
            http.client.HTTPException, zipfile.BadZipFile):
        # Parser and filesystem exceptions can contain private source data.
        print("Export processing stopped. No source details are logged. Any existing progress was retained.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
