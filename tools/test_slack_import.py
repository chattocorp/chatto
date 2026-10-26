"""Exercise export parsing with synthetic data only; never use a private export."""

import importlib.util
import json
from pathlib import Path
import tempfile
import sys
import unittest
import zipfile
import contextlib
import io
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("slack_import", Path(__file__).with_name("slack-import.py"))
slack = importlib.util.module_from_spec(spec)
spec.loader.exec_module(slack)


class ExportTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / "export.zip"
        self.root = {"type": "message", "user": "U1", "text": "Hello <@U2>",
                     "ts": "1766946098.872519", "thread_ts": "1766946098.872519",
                     "reply_count": 1, "files": [{"id": "F1", "mode": "hidden_by_limit"}]}
        self.reply = {"type": "message", "user": "U2", "text": "Hi",
                      "ts": "1766946098.872520", "thread_ts": self.root["ts"],
                      "reactions": [{"name": "heart", "count": 1, "users": ["U1"]}]}
        self.data = {
            "users.json": [{"id": "U1", "name": "one"}, {"id": "U2", "name": "two", "deleted": True}],
            "channels.json": [{"id": "C1", "name": "general", "members": ["U1", "U2"]}],
            "general/2025-12-28.json": [self.reply, self.root],
            "lists.json": [""], "huddle_transcripts.json": [""],
        }

    def archive(self):
        with zipfile.ZipFile(self.path, "w") as output:
            for name, value in self.data.items():
                output.writestr(name, json.dumps(value))

    def directory(self):
        root = Path(self.temp.name) / "source"
        root.mkdir(exist_ok=True)
        for name, value in self.data.items():
            entry = root / name
            entry.parent.mkdir(exist_ok=True)
            entry.write_text(json.dumps(value))
        return root

    def test_directory_matches_zip_and_binds_source_changes(self):
        self.archive()
        root = self.directory()
        plan = slack.read_export(root)
        zipped = slack.read_export(self.path)
        self.assertEqual(plan["summary"], zipped["summary"])
        self.assertEqual(plan["messages"], zipped["messages"])
        progress = slack.progress_directory(root)
        self.assertEqual(progress.parent, root.resolve())
        progress.mkdir()
        (progress / "journal.json").write_text("private progress")
        self.assertEqual(plan, slack.read_export(root))
        self.reply["text"] = "changed"
        self.directory()
        self.assertNotEqual(plan["archive_sha256"], slack.read_export(root)["archive_sha256"])

    def test_directory_rejects_symlinks_and_size_limit(self):
        root = self.directory()
        with self.assertRaisesRegex(slack.InvalidExport, "inspection limit"):
            slack.read_export(root, max_bytes=1)
        (root / "outside").symlink_to(Path(self.temp.name), target_is_directory=True)
        with self.assertRaisesRegex(slack.InvalidExport, "symbolic link"):
            slack.read_export(root)

    def test_unicode_directory_names_match_channel_metadata(self):
        self.data["channels.json"][0]["name"] = "caf\u00e9"
        self.data["cafe\u0301/2025-12-28.json"] = self.data.pop("general/2025-12-28.json")
        self.archive()
        for source in (self.path, self.directory()):
            plan = slack.read_export(source)
            self.assertEqual(plan["summary"]["messages"], 2)
            self.assertNotIn("unhandled_archive_entries", plan["summary"]["issues"])

    def test_nested_attachment_files_are_included_in_report(self):
        self.reply["attachments"] = [{"fallback": "Shared file", "files": [{"id": "F2"}]}]
        self.archive()
        plan = slack.read_export(self.path)
        self.assertEqual(plan["summary"]["files"], 2)
        self.assertEqual(plan["summary"]["issues"]["file_download_or_local_mapping_required"], 1)
        report = slack.preflight(plan)
        self.assertTrue(report["conversion_assumes_attachment_summaries"])
        self.assertEqual(report["convertible_messages"], 1)

    def test_preflight_has_no_operator_access_or_private_content(self):
        root = self.directory()
        before = sorted(root.rglob("*"))
        output = io.StringIO()
        with patch.object(sys, "argv", ["slack-import", str(root), "--preflight"]), \
                patch.object(slack, "Operator") as operator, contextlib.redirect_stdout(output):
            self.assertEqual(slack.main(), 0)
        operator.assert_not_called()
        self.assertEqual(before, sorted(root.rglob("*")))
        report = json.loads(output.getvalue())
        self.assertEqual(report["convertible_messages"], 1)
        self.assertEqual(sum(report["conversion_failures"].values()), 1)
        for private in ("Hello", "U1", "general", "one"):
            self.assertNotIn(private, output.getvalue())

    def test_saved_report_stays_inside_source_and_does_not_change_digest(self):
        root = self.directory()
        before = slack.read_export(root)["archive_sha256"]
        with patch.object(sys, "argv", ["slack-import", str(root), "--preflight", "--save-report"]), \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(slack.main(), 0)
        report = root / ".chatto-import" / "preflight.json"
        self.assertTrue(report.is_file())
        self.assertEqual(report.stat().st_mode & 0o777, 0o600)
        self.assertEqual(before, slack.read_export(root)["archive_sha256"])

    def test_api_preflight_reports_counts_without_writes_or_secrets(self):
        for user in self.data["users.json"]:
            user["team_id"] = "T1"
        root = self.directory()
        before = sorted(root.rglob("*"))
        output = io.StringIO()
        with patch.object(sys, "argv", ["slack-import", str(root), "--preflight", "--slack-token-stdin"]), \
                patch.object(sys, "stdin", io.StringIO("secret-token")), \
                patch.object(slack, "SlackAPI") as api, patch.object(slack, "Operator") as operator, \
                contextlib.redirect_stdout(output):
            client = api.return_value
            client.scopes = {"users:read", "users:read.email"}
            client.call.return_value = {"team_id": "T1"}
            client.users.return_value = {"U1": {"id": "U1", "team_id": "T1", "profile": {"email": "one@example.org"}}}
            self.assertEqual(slack.main(), 0)
        operator.assert_not_called()
        self.assertEqual(before, sorted(root.rglob("*")))
        self.assertEqual(json.loads(output.getvalue())["slack_api"]["active_humans_with_email"], 1)
        for private in ("secret-token", "one@example.org", "U1", "T1"):
            self.assertNotIn(private, output.getvalue())

    def test_offline_plan_keeps_identity_enrichment_opt_in(self):
        self.data["users.json"][0].update(team_id="T1", profile={"email": "one@example.org"})
        self.archive()
        offline = slack.read_export(self.path)
        self.assertNotIn("source_email", offline["users"][0])
        self.assertNotIn("slack_workspace_ids", offline)
        online = slack.read_export(self.path, include_slack_identity=True)
        self.assertEqual(online["slack_workspace_ids"], ["T1"])
        self.assertEqual(online["users"][0]["source_email"], "one@example.org")

    def test_source_order_fidelity_and_gaps(self):
        self.archive()
        plan = slack.read_export(self.path)
        summary = plan["summary"]
        self.assertEqual(plan["messages"][0]["created_at"], "2025-12-28T18:21:38.872519Z")
        self.assertEqual(plan["messages"][1]["created_at"], "2025-12-28T18:21:38.872520Z")
        self.assertEqual(plan["messages"][0]["source"], self.root)
        self.assertEqual(summary["thread_roots"], 1)
        self.assertEqual(summary["thread_replies"], 1)
        self.assertEqual(summary["reactions"], 1)
        self.assertEqual(summary["issues"]["file_hidden_by_limit"], 1)
        self.assertNotIn("incomplete_thread_replies", summary["issues"])
        self.assertNotIn("Hello", json.dumps(summary))

    def test_archive_digest_changes_with_source(self):
        self.archive()
        before = slack.read_export(self.path)["archive_sha256"]
        self.reply["text"] = "Changed"
        self.archive()
        self.assertNotEqual(before, slack.read_export(self.path)["archive_sha256"])

    def test_incomplete_history_is_reported(self):
        self.data["general/2025-12-28.json"] = [self.reply]
        self.reply["reactions"][0]["count"] = 2
        self.reply["user"] = "UNKNOWN"
        self.data["private/2025-12-28.json"] = []
        self.archive()
        summary = slack.read_export(self.path)["summary"]
        for issue in ("missing_thread_root", "incomplete_reaction_users",
                      "unresolved_message_author", "unhandled_archive_entries"):
            self.assertEqual(summary["issues"][issue], 1)

    def test_duplicate_messages_rejected(self):
        self.data["general/2025-12-28.json"].append(self.root)
        self.archive()
        with self.assertRaises(slack.InvalidExport):
            slack.read_export(self.path)

    def test_non_message_record_rejected(self):
        self.reply["type"] = "unknown_event"
        self.archive()
        with self.assertRaisesRegex(slack.InvalidExport, "record type"):
            slack.read_export(self.path)

    def test_reply_cannot_be_a_thread_root(self):
        nested = dict(self.reply, ts="1766946098.872521", thread_ts=self.reply["ts"])
        self.data["general/2025-12-28.json"].append(nested)
        self.archive()
        self.assertEqual(slack.read_export(self.path)["summary"]["issues"]["invalid_thread_root"], 1)

    def test_timestamp_rejects_float_and_invalid_fraction(self):
        for value in (1766946098.872519, "1766946098.1", "bad"):
            with self.assertRaises(slack.InvalidExport):
                slack.timestamp(value)

    def test_duplicate_zip_entries_rejected(self):
        self.archive()
        import warnings
        with warnings.catch_warnings():
            warnings.simplefilter("ignore", UserWarning)
            with zipfile.ZipFile(self.path, "a") as output:
                output.writestr("users.json", "[]")
        with self.assertRaises(slack.InvalidExport):
            slack.read_export(self.path)


if __name__ == "__main__":
    unittest.main()
