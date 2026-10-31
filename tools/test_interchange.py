# /// script
# requires-python = ">=3.11"
# dependencies = ["jsonschema[format]==4.26.0"]
# ///
"""Check draft interchange schemas and synthetic fixtures, without server access.

The semantic checker below is a conformance-test oracle, not the production
archive reader. It keeps small fixtures in memory and does not implement import,
destination limits, crash recovery, or export consistency.
"""

import copy
from datetime import datetime
import hashlib
import json
from pathlib import Path
import unittest

from jsonschema import Draft202012Validator, FormatChecker, ValidationError
from referencing import Registry, Resource


ROOT = Path(__file__).resolve().parents[1]
SCHEMA_DIR = ROOT / "schema/interchange/v1"
EXAMPLE = SCHEMA_DIR / "examples/conversation"
SCHEMAS = {p.stem.removesuffix(".schema"): json.loads(p.read_text())
           for p in SCHEMA_DIR.glob("*.schema.json")}
REGISTRY = Registry().with_resources(
    (schema["$id"], Resource.from_contents(schema)) for schema in SCHEMAS.values())
VALIDATORS = {name: Draft202012Validator(schema, registry=REGISTRY,
                                        format_checker=FormatChecker())
              for name, schema in SCHEMAS.items()}


def require(condition, code):
    """Use content-free errors; even test helpers must not print source records."""
    if not condition:
        raise ValueError(code)


def timestamp(value):
    """Compare UTC instants without losing the format's nanosecond precision."""
    whole, _, fraction = value.removesuffix("Z").partition(".")
    return datetime.fromisoformat(whole), fraction.ljust(9, "0")


def unique(rows):
    result = {row["id"]: row for row in rows}
    require(len(result) == len(rows), "duplicate_id")
    return result


def validate_records(records):
    """Exercise cross-record rules shared by the example and negative cases."""
    VALIDATORS["manifest"].validate(records["manifest"])
    for plural, singular in (("users", "user"), ("rooms", "room"),
                             ("assets", "asset"), ("events", "event")):
        for record in records[plural]:
            VALIDATORS[singular].validate(record)
    users, rooms, assets = (unique(records[k]) for k in ("users", "rooms", "assets"))
    unique(records["events"])
    manifest = records["manifest"]
    partial = manifest["history"] == "partial"
    omissions = {item["category"] for item in manifest["omissions"]}
    require(not partial or "history" in omissions, "missing_history_omission")
    cut = timestamp(manifest["captured_at"]) if manifest["captured_at"] else None
    require(cut is not None or manifest["consistency"] == "unknown", "unknown_capture_cut")
    require(manifest["consistency"] != "unknown" or partial, "uncertain_complete_history")
    messages, threads, member_state, membership_final, metadata = {}, set(), {}, set(), {}

    def user(key):
        require(key in users, "unknown_user")

    def content(value):
        if value["status"] == "available":
            require(value["body"]["text"].strip() or value["asset_ids"], "empty_content")
            require(all(key in assets for key in value["asset_ids"]), "unknown_asset")
        elif value["status"] == "unavailable":
            require("content" in omissions, "missing_content_omission")

    for asset in assets.values():
        if asset["storage"]["status"] == "unavailable":
            require("files" in omissions, "missing_files_omission")

    for identity in users.values():
        if "avatar_asset_id" in identity:
            require(identity["avatar_asset_id"] in assets, "unknown_avatar_asset")

    for event in records["events"]:
        kind, room, data = event["type"], event["room_id"], event["data"]
        require(room in rooms, "unknown_room")
        if event["actor_id"] is not None:
            user(event["actor_id"])
        if event["occurred_at"] is not None:
            require(cut is None or timestamp(event["occurred_at"]) <= cut, "event_after_capture")

        if kind in {"message.posted", "message.snapshot"}:
            key = data["message_id"]
            snapshot = kind == "message.snapshot"
            previous = messages.get(key)
            require(not previous or snapshot, "duplicate_message")
            require(not snapshot or partial, "snapshot_requires_partial_history")
            user(data["author_id"])
            for field in ("thread_id", "reply_to"):
                target = data[field]
                if target is not None:
                    require(target in messages, "forward_message_reference")
                    require(messages[target]["room"] == room, "cross_room_reference")
                    if field == "thread_id":
                        require(messages[target]["thread_id"] is None, "nested_thread")
                        threads.add(target)
            content(data["content"])
            require(snapshot or data["content"]["status"] != "deleted", "deleted_post")
            created = data["created_at"] if snapshot else event["occurred_at"]
            state = {"room": room, "author_id": data["author_id"],
                     "thread_id": data["thread_id"], "reply_to": data["reply_to"],
                     "created_at": created, "snapshot": snapshot,
                     "deleted": data["content"]["status"] == "deleted",
                     "versions": [], "reactions": {}}
            if previous:
                require(not previous["snapshot"], "duplicate_snapshot")
                for field in ("room", "author_id", "thread_id", "reply_to"):
                    require(previous[field] == state[field], "snapshot_identity_changed")
                require(created is not None and timestamp(created) == timestamp(previous["created_at"]),
                        "snapshot_creation_changed")
                require(not previous["deleted"] or state["deleted"], "snapshot_resurrection")
                state["versions"] = previous["versions"]
            if snapshot:
                for field in ("created_at", "edited_at", "deleted_at"):
                    if data[field] is not None:
                        require(cut is None or timestamp(data[field]) <= cut, "snapshot_after_capture")
                for field in ("edited_at", "deleted_at"):
                    if created is not None and data[field] is not None:
                        require(timestamp(data[field]) >= timestamp(created), "mutation_before_creation")
                if data["edited_at"] and data["deleted_at"]:
                    require(timestamp(data["edited_at"]) <= timestamp(data["deleted_at"]), "edit_after_deletion")
                require(state["deleted"] or data["deleted_at"] is None, "nondeleted_with_deletion_time")
                emojis = set()
                for reaction in data["reactions"]:
                    require(reaction["emoji"] not in emojis, "duplicate_reaction_group")
                    emojis.add(reaction["emoji"])
                    for reactor in reaction["user_ids"]:
                        user(reactor)
                        state["reactions"][(reaction["emoji"], reactor)] = True
            if data["content"]["status"] != "deleted":
                state["versions"].append(data["content"])
            messages[key] = state
        elif kind.startswith(("message.", "reaction.")):
            require(data["message_id"] in messages, "forward_message_reference")
            state = messages[data["message_id"]]
            require(state["room"] == room, "cross_room_reference")
            require(not state["snapshot"], "mutation_after_snapshot")
            if kind in {"message.edited", "message.deleted"} and state["created_at"]:
                require(timestamp(event["occurred_at"]) >= timestamp(state["created_at"]), "mutation_before_creation")
            if kind == "message.edited":
                require(not state["deleted"], "edit_after_deletion")
                require(data["content"]["status"] != "deleted", "deleted_edit")
                content(data["content"])
                state["versions"].append(data["content"])
            elif kind == "message.deleted":
                require(not state["deleted"], "duplicate_deletion")
                state["deleted"] = True
            else:
                user(data["user_id"])
                key = (data["emoji"], data["user_id"])
                wanted = kind == "reaction.added"
                old = state["reactions"].get(key, None if partial else False)
                require(old != wanted, "invalid_reaction_transition")
                state["reactions"][key] = wanted
        elif kind == "thread.created":
            root = data["root_id"]
            require(root in messages, "forward_message_reference")
            require(messages[root]["room"] == room and messages[root]["thread_id"] is None,
                    "invalid_thread_root")
            require(root not in threads, "duplicate_thread")
            threads.add(root)
        elif kind.startswith("room.member"):
            require(room not in membership_final, "membership_after_snapshot")
            state = member_state.setdefault(room, {})
            if kind == "room.membership_snapshot":
                final = set(data["user_ids"])
                for key in final:
                    user(key)
                if not partial:
                    require(final == {key for key, present in state.items() if present},
                            "membership_mismatch")
                membership_final.add(room)
            else:
                user(data["user_id"])
                wanted = kind == "room.member_joined"
                require(state.get(data["user_id"], None if partial else False) != wanted,
                        "invalid_membership_transition")
                state[data["user_id"]] = wanted
                emojis = set()
                for reaction in data.get("reactions", []):
                    require(reaction["emoji"] not in emojis, "duplicate_reaction_group")
                    emojis.add(reaction["emoji"])
                    for reactor in reaction["user_ids"]:
                        user(reactor)
        else:
            field = {"room.name_changed": "name", "room.topic_changed": "topic",
                     "room.purpose_changed": "purpose", "room.archived": "archived",
                     "room.unarchived": "archived"}[kind]
            metadata[(room, field)] = data.get("value", kind == "room.archived")
    require(membership_final == set(rooms), "missing_membership_snapshot")
    if not partial:
        require(all(rooms[room][field] == value for (room, field), value in metadata.items()),
                "metadata_mismatch")
    for state in messages.values():
        if state["deleted"]:
            require(all(v["status"] == "unavailable" for v in state["versions"]),
                    "deleted_content_exposed")


def example():
    records = {"manifest": json.loads((EXAMPLE / "manifest.json").read_text())}
    for name in ("users", "rooms", "assets", "events"):
        records[name] = [json.loads(line) for line in (EXAMPLE / (name + ".jsonl")).read_text().splitlines()]
    return records


class InterchangeContractTests(unittest.TestCase):
    def setUp(self):
        self.records = example()

    def test_schemas_and_complete_example(self):
        for schema in SCHEMAS.values():
            Draft202012Validator.check_schema(schema)
        validate_records(self.records)

    def test_example_file_bytes_match_declared_integrity(self):
        paths = set()
        for asset in self.records["assets"]:
            storage = asset["storage"]
            path = EXAMPLE / storage["path"]
            self.assertFalse(path.is_symlink())
            self.assertEqual(path.stat().st_nlink, 1)
            self.assertNotIn(path, paths)
            paths.add(path)
            raw = path.read_bytes()
            self.assertEqual(len(raw), storage["size"])
            self.assertEqual(hashlib.sha256(raw).hexdigest(), storage["sha256"])
        self.assertEqual(paths, set((EXAMPLE / "files").iterdir()))

    def test_unknown_versions_types_fields_and_invalid_dates_fail(self):
        for change in (
            lambda r: r["manifest"].update(version=2),
            lambda r: r["manifest"].update(export_id="not-a-uuid"),
            lambda r: r["events"][0].update(type="room.unknown"),
            lambda r: r["events"][0].update(secret="not permitted"),
            lambda r: r["events"][0].update(occurred_at="2020-02-30T00:00:00Z"),
            lambda r: r["events"][-1].update(occurred_at="2020-03-15T00:00:00Z"),
        ):
            with self.subTest(change=change):
                records = copy.deepcopy(self.records)
                change(records)
                with self.assertRaises(ValidationError):
                    validate_records(records)

    def test_asset_paths_cannot_escape_or_fetch_remote_content(self):
        for path in ("../secret", "/tmp/file", "files/../secret", "files/a/b", "files/a\\b", "https://example.com/file"):
            with self.subTest(path=path):
                self.records["assets"][0]["storage"]["path"] = path
                with self.assertRaises(ValidationError):
                    validate_records(self.records)

    def test_reference_and_history_failures(self):
        cases = [
            ("duplicate_id", lambda r: r["users"].append(r["users"][0])),
            ("unknown_user", lambda r: r["events"][0]["data"].update(user_id="absent")),
            ("unknown_asset", lambda r: r["events"][2]["data"]["content"].update(asset_ids=["absent"])),
            ("unknown_avatar_asset", lambda r: r["users"][0].update(avatar_asset_id="absent")),
            ("forward_message_reference", lambda r: r["events"][4]["data"].update(thread_id="future")),
            ("missing_membership_snapshot", lambda r: r["events"].pop()),
            ("snapshot_requires_partial_history", lambda r: r["manifest"].update(history="complete")),
            ("event_after_capture", lambda r: r["events"][0].update(occurred_at="2021-01-01T00:00:00Z")),
            ("mutation_before_creation", lambda r: r["events"][5].update(occurred_at="2020-03-13T00:00:00Z")),
            ("nondeleted_with_deletion_time", lambda r: r["events"][7]["data"].update(content={"status": "unavailable", "reason": "unknown"}, deleted_at="2020-03-14T13:00:00Z")),
        ]
        for code, change in cases:
            with self.subTest(code=code):
                records = copy.deepcopy(self.records)
                records["manifest"]["omissions"].append({"category": "content", "reason": "Test", "count": None})
                change(records)
                with self.assertRaisesRegex(ValueError, code):
                    validate_records(records)

    def test_snapshot_is_terminal_but_allows_later_replies(self):
        event = copy.deepcopy(self.records["events"][4])
        event.update(id="late-reply")
        event["data"].update(message_id="late-message", thread_id="message-3", reply_to="message-3")
        self.records["events"].append(event)
        validate_records(self.records)
        event = copy.deepcopy(self.records["events"][6])
        event.update(id="late-reaction")
        event["data"]["message_id"] = "message-3"
        self.records["events"].append(event)
        with self.assertRaisesRegex(ValueError, "mutation_after_snapshot"):
            validate_records(self.records)

    def test_membership_snapshot_cannot_invent_a_join_time(self):
        event = copy.deepcopy(self.records["events"][0])
        event.update(id="late-join")
        self.records["events"].append(event)
        with self.assertRaisesRegex(ValueError, "membership_after_snapshot"):
            validate_records(self.records)

    def test_complete_history_matches_final_membership(self):
        self.records["events"] = [e for e in self.records["events"] if e["type"] != "message.snapshot"]
        self.records["manifest"].update(history="complete", omissions=[])
        validate_records(self.records)
        self.records["events"][-1]["data"]["user_ids"].append("user-2")
        with self.assertRaisesRegex(ValueError, "membership_mismatch"):
            validate_records(self.records)

    def test_deleted_history_cannot_export_old_plaintext(self):
        event = copy.deepcopy(self.records["events"][5])
        event.update(id="delete", type="message.deleted", data={"message_id": "message-2"})
        self.records["events"].append(event)
        with self.assertRaisesRegex(ValueError, "deleted_content_exposed"):
            validate_records(self.records)
        for event in self.records["events"]:
            if event["data"].get("message_id") == "message-2" and "content" in event["data"]:
                event["data"]["content"] = {"status": "unavailable", "reason": "not_retained"}
        self.records["manifest"]["omissions"].append({"category": "content", "reason": "Erased", "count": 2})
        validate_records(self.records)

    def test_equal_timestamps_keep_file_order_and_nanoseconds(self):
        self.records["events"][3]["occurred_at"] = "2020-03-14T12:03:00.000000001Z"
        self.records["events"][4]["occurred_at"] = "2020-03-14T12:03:00.000000001Z"
        validate_records(self.records)
        self.assertLess(timestamp("2020-03-14T12:03:00.000000001Z"),
                        timestamp("2020-03-14T12:03:00.000000002Z"))

    def test_every_event_variant_has_a_valid_contract_example(self):
        variants = {
            "reaction.removed": {"message_id": "message-1", "user_id": "user-2", "emoji": "❤️"},
            "room.name_changed": {"value": "renamed"},
            "room.topic_changed": {"value": "New topic"},
            "room.purpose_changed": {"value": "New purpose"},
            "room.archived": {},
            "room.unarchived": {},
            "message.deleted": {"message_id": "message-2"},
        }
        for kind, data in variants.items():
            self.records["events"].append({
                "id": "example-" + kind, "type": kind, "room_id": "room-1",
                "actor_id": None, "occurred_at": "2020-03-14T13:00:00Z", "data": data})
        for event in self.records["events"]:
            if event["data"].get("message_id") == "message-2" and "content" in event["data"]:
                event["data"]["content"] = {"status": "unavailable", "reason": "not_retained"}
        self.records["manifest"]["omissions"].append({"category": "content", "reason": "Erased", "count": 2})
        validate_records(self.records)
        covered = {e["type"] for e in self.records["events"]}
        self.assertEqual(covered, set(SCHEMAS["event"]["properties"]["type"]["enum"]))

    def test_snapshot_cannot_rewrite_author_or_known_creation_time(self):
        posted = self.records["events"][2]
        snapshot = copy.deepcopy(posted)
        snapshot.update(id="final-message", type="message.snapshot", occurred_at=None, actor_id=None)
        snapshot["data"].update(created_at=posted["occurred_at"], edited_at=None,
                                deleted_at=None, reactions=[])
        self.records["events"].append(snapshot)
        validate_records(self.records)
        for field, value, error in (("author_id", "user-2", "snapshot_identity_changed"),
                                    ("created_at", None, "snapshot_creation_changed")):
            records = copy.deepcopy(self.records)
            records["events"][-1]["data"][field] = value
            with self.assertRaisesRegex(ValueError, error):
                validate_records(records)

    def test_cross_room_reference_and_duplicate_reaction_fail(self):
        other = dict(self.records["rooms"][0], id="room-2", name="other")
        self.records["rooms"].append(other)
        self.records["events"][4]["room_id"] = "room-2"
        with self.assertRaisesRegex(ValueError, "cross_room_reference"):
            validate_records(self.records)
        self.records = example()
        duplicate = copy.deepcopy(self.records["events"][6])
        duplicate["id"] = "duplicate-reaction"
        self.records["events"].append(duplicate)
        with self.assertRaisesRegex(ValueError, "invalid_reaction_transition"):
            validate_records(self.records)

    def test_unknown_membership_start_does_not_require_a_fake_join(self):
        self.records["events"] = [e for e in self.records["events"]
                                  if e["type"] != "room.member_joined"]
        validate_records(self.records)

    def test_foreign_export_can_declare_unknown_capture_cut(self):
        self.records["manifest"].update(captured_at=None, consistency="unknown")
        validate_records(self.records)
        self.records["manifest"]["consistency"] = "consistent"
        with self.assertRaisesRegex(ValueError, "unknown_capture_cut"):
            validate_records(self.records)


if __name__ == "__main__":
    unittest.main()
