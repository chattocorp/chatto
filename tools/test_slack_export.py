"""Synthetic Slack-to-interchange contracts; no private source or network access."""

import contextlib
from concurrent.futures import ThreadPoolExecutor
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

from slack_apply import ApplyError
from slack_files import FileImports
from test_interchange import validate_records

spec = importlib.util.spec_from_file_location("slack_export", Path(__file__).with_name("slack-export.py"))
exporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(exporter)


class SlackArchiveTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "source"
        self.source.mkdir()
        self.data = {
            "users.json": [{"id": "U1", "name": "alice", "team_id": "T1", "profile": {"email": "alice@example.com"}},
                           {"id": "U2", "name": "b", "team_id": "T1"}],
            "channels.json": [{"id": "C1", "name": "general", "members": ["U1"], "is_archived": True,
                               "topic": {"value": "Topic"}, "purpose": {"value": "Purpose"}}],
            "general/2020-01-01.json": [
                {"type": "message", "subtype": "channel_join", "user": "U1", "text": "joined", "ts": "1577836800.000000"},
                {"type": "message", "user": "U1", "text": "Edited root", "ts": "1577836801.000001",
                 "edited": {"ts": "1577836810.000000"}, "thread_ts": "1577836801.000001", "reply_count": 1,
                 "reactions": [{"name": "heart", "users": ["U2"], "count": 1}]},
                {"type": "message", "user": "U2", "text": "Reply", "ts": "1577836802.000000", "thread_ts": "1577836801.000001"},
                {"type": "message", "subtype": "tombstone", "user": "U1", "text": "deleted", "ts": "1577836803.000000",
                 "reactions": [{"name": "thumbsup", "users": ["U2"], "count": 1}]},
                {"type": "message", "subtype": "channel_leave", "user": "U2", "text": "left", "ts": "1577836804.000000"},
                {"type": "message", "subtype": "channel_topic", "user": "U1", "topic": "Topic", "ts": "1577836805.000000"},
            ]}

    def plan(self):
        for name, data in self.data.items():
            path = self.source / name
            path.parent.mkdir(exist_ok=True)
            path.write_text(json.dumps(data))
        return exporter.source.read_export(self.source, include_slack_identity=True, include_interchange=True)

    def test_history_and_identity_round_trip_to_contract(self):
        records, copies = exporter.build_archive(self.plan())
        validate_records(records)
        self.assertEqual(copies, {})
        self.assertEqual(records["users"][0]["emails"], ["alice@example.com"])
        self.assertEqual(records["users"][1]["username"], "b")
        self.assertTrue(records["rooms"][0]["archived"])
        self.assertEqual(records["rooms"][0]["purpose"], "Purpose")
        root, reply, deleted = records["events"][1:4]
        self.assertEqual(root["type"], "message.snapshot")
        self.assertIsNone(root["occurred_at"])
        self.assertEqual(root["data"]["edited_at"], "2020-01-01T00:00:10.000000Z")
        self.assertEqual(reply["data"]["thread_id"], root["data"]["message_id"])
        self.assertEqual(deleted["data"]["content"], {"status": "deleted"})
        self.assertEqual(deleted["data"]["reactions"][0]["user_ids"], ["U2"])
        self.assertEqual(records["events"][0]["type"], "room.member_joined")
        self.assertEqual(records["events"][-1]["data"], {"user_ids": ["U1"]})
        again, _ = exporter.build_archive(self.plan())
        self.assertEqual(records, again)
        self.data["users.json"][0]["profile"]["email"] = "new@example.com"
        changed, _ = exporter.build_archive(self.plan())
        self.assertNotEqual(records["manifest"]["export_id"], changed["manifest"]["export_id"])

    def with_file(self):
        item = {"id": "F1", "name": "hello.txt", "mimetype": "text/plain", "size": 5,
                "url_private_download": "https://files.slack.com/private?token=do-not-copy"}
        self.data["general/2020-01-01.json"][1]["files"] = [item]
        return item

    def downloads(self, plan):
        cache = self.root / "cached"
        cache.write_bytes(b"hello")
        class Cache:
            def get(inner, item):
                return {"path": str(cache), "filename": "hello.txt", "content_type": "text/plain",
                        "size": 5, "sha256": hashlib.sha256(b"hello").hexdigest()}
        return FileImports(plan, Cache())

    def test_available_shared_file_bytes_and_private_output(self):
        item = self.with_file()
        self.data["general/2020-01-01.json"][2]["files"] = [item]
        plan = self.plan()
        records, copies = exporter.build_archive(plan, self.downloads(plan))
        validate_records(records)
        self.assertEqual(len(records["assets"]), 1)
        self.assertEqual(len(copies), 1)
        self.assertNotIn("do-not-copy", json.dumps(records))
        output = self.root / "output"
        exporter.write_archive(output, records, copies)
        self.assertEqual(next((output / "files").iterdir()).read_bytes(), b"hello")
        self.assertEqual(output.stat().st_mode & 0o777, 0o700)
        for path in output.rglob("*"):
            if path.is_file():
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
        with self.assertRaisesRegex(ApplyError, "already exists"):
            exporter.write_archive(output, records, copies)

    def test_missing_file_is_an_asset_not_fake_message_text(self):
        self.with_file()
        plan = self.plan()
        with self.assertRaisesRegex(ApplyError, "unavailable"):
            exporter.build_archive(plan)
        records, copies = exporter.build_archive(plan, allow_missing_files=True)
        validate_records(records)
        self.assertFalse(copies)
        self.assertEqual(records["assets"][0]["storage"]["status"], "unavailable")
        self.assertEqual(records["events"][1]["data"]["content"]["asset_ids"], ["F1"])
        self.assertNotIn("Attachment unavailable", json.dumps(records))

    def test_file_change_leaves_no_completion_manifest(self):
        self.with_file()
        plan = self.plan()
        records, copies = exporter.build_archive(plan, self.downloads(plan))
        (self.root / "cached").write_bytes(b"other")
        output = self.root / "output"
        with self.assertRaisesRegex(ApplyError, "changed"):
            exporter.write_archive(output, records, copies)
        self.assertFalse((output / "manifest.json").exists())

    def test_existing_output_and_bad_content_precede_downloads(self):
        self.with_file()
        self.plan()
        with patch.object(sys, "argv", ["slack-export.py", str(self.source), "--output", str(self.source)]), \
                patch.object(exporter, "FileCache") as cache, contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(exporter.main(), 1)
            cache.assert_not_called()
        self.data["general/2020-01-01.json"][1]["blocks"] = [{"type": "unsupported"}]
        self.plan()
        with patch.object(sys, "argv", ["slack-export.py", str(self.source), "--output", str(self.root / "output")]), \
                patch.object(exporter, "FileCache") as cache, contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(exporter.main(), 1)
            cache.assert_not_called()

    def test_source_output_location_and_cli_completion(self):
        self.plan()
        with patch.object(sys, "argv", ["slack-export.py", str(self.source), "--output", str(self.source / "bad")]), \
                contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(exporter.main(), 1)
        progress = self.source / ".chatto-import"
        progress.mkdir(mode=0o700)
        output = progress / "archive"
        with patch.object(sys, "argv", ["slack-export.py", str(self.source), "--output", str(output)]), \
                contextlib.redirect_stdout(io.StringIO()) as stdout:
            self.assertEqual(exporter.main(), 0)
        self.assertEqual(json.loads(stdout.getvalue())["events"], 7)
        # Output under the reserved directory does not change source identity.
        self.assertEqual(json.loads((output / "manifest.json").read_text())["export_id"],
                         exporter.build_archive(self.plan())[0]["manifest"]["export_id"])

    def test_literal_mentions_and_source_account_states(self):
        self.data["users.json"][1].update(deleted=True, is_bot=True)
        self.data["general/2020-01-01.json"][1]["blocks"] = [{"type": "rich_text", "elements": [
            {"type": "rich_text_section", "elements": [{"type": "user", "user_id": "U2"}]}]}]
        records, _ = exporter.build_archive(self.plan())
        self.assertEqual(records["users"][1]["status"], "deactivated")
        self.assertEqual(records["users"][1]["kind"], "bot")
        self.assertEqual(records["events"][1]["data"]["content"]["body"]["text"], "`@b`")
        validate_records(records)

    def test_unknown_custom_reaction_fails(self):
        self.data["general/2020-01-01.json"][1]["reactions"][0]["name"] = "custom_company_emoji"
        with self.assertRaisesRegex(ApplyError, "unknown reaction"):
            exporter.build_archive(self.plan())

    def test_unresolved_reference_and_invalid_email_fail(self):
        plan = self.plan()
        plan["messages"][1]["source_author_id"] = "unknown"
        with self.assertRaisesRegex(ApplyError, "Unresolved message"):
            exporter.build_archive(plan)
        self.data["users.json"][0]["profile"]["email"] = "invalid"
        with self.assertRaisesRegex(ApplyError, "schema"):
            exporter.build_archive(self.plan())

    def test_notice_does_not_guess_from_text(self):
        del self.data["general/2020-01-01.json"][-1]["topic"]
        self.data["general/2020-01-01.json"][-1]["text"] = "private text"
        with self.assertRaisesRegex(ApplyError, "structured metadata"):
            exporter.build_archive(self.plan())

    def test_private_channel_is_not_made_public(self):
        self.data["channels.json"][0]["is_private"] = True
        records, _ = exporter.build_archive(self.plan())
        self.assertEqual(records["rooms"][0]["visibility"], "private")

    def test_repeated_membership_transition_fails(self):
        self.data["general/2020-01-01.json"].append({"type": "message", "subtype": "channel_join",
                                                  "user": "U1", "ts": "1577836900.000000"})
        with self.assertRaisesRegex(ApplyError, "repeated transition"):
            exporter.build_archive(self.plan())

    def test_competing_output_writers_do_not_overwrite(self):
        records, copies = exporter.build_archive(self.plan())
        output = self.root / "output"
        def write():
            try:
                exporter.write_archive(output, records, copies)
                return True
            except ApplyError:
                return False
        with ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(lambda _: write(), range(2)))
        self.assertEqual(sorted(results), [False, True])
        self.assertEqual(json.loads((output / "manifest.json").read_text()), records["manifest"])

    def test_legacy_reader_keeps_its_journal_binding_shape(self):
        self.plan()
        legacy = exporter.source.read_export(self.source, include_slack_identity=True)
        self.assertNotIn("private", legacy["rooms"][0])


if __name__ == "__main__":
    unittest.main()
