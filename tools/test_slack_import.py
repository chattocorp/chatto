"""Exercise export parsing with synthetic data only; never use a private export."""

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import zipfile

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
