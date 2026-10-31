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
    def test_avatar_asset_in_archive(self):
        from types import SimpleNamespace
        self.data['users.json'][0]['profile']['image_512'] = 'https://avatars.slack-edge.com/example'
        plan = self.plan()
        ready = {'filename':'avatar','content_type':'image/png','size':3,'sha256':hashlib.sha256(b'png').hexdigest(),'path':'cached'}
        records, copies = exporter.build_archive(plan, avatars=SimpleNamespace(ready={'U1':ready}))
        validate_records(records)
        self.assertEqual(records['users'][0]['avatar_asset_id'], 'avatar:U1')
        self.assertEqual(len(copies),1)
        self.assertFalse(any('avatars' in o['reason'] for o in records['manifest']['omissions']))

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

    def test_source_bot_identities_complete_catalog(self):
        rows = self.data["general/2020-01-01.json"]
        rows[1].update(user="USLACKBOT", bot_id="B1", subtype="slackbot_response")
        rows[3]["user"] = "USLACKBOT"
        rows[2].pop("user")
        rows[2].update(bot_id="B2", username="Legacy integration", subtype="bot_message")
        records, _ = exporter.build_archive(self.plan())
        validate_records(records)
        bots = {u["id"]: u for u in records["users"] if u["kind"] == "bot"}
        self.assertEqual(set(bots), {"USLACKBOT", "slack-bot:B2"})
        self.assertEqual(bots["slack-bot:B2"]["display_name"], "Legacy integration")
        self.assertTrue(all(not bot["emails"] for bot in bots.values()))
        self.assertEqual(records, exporter.build_archive(self.plan())[0])

    def test_bot_profile_reuses_existing_account(self):
        self.data["users.json"][1].update(is_bot=True, deleted=True, profile={"bot_id": "B2"})
        row = self.data["general/2020-01-01.json"][2]
        row.pop("user")
        row.update(bot_id="B2", username="Bot name", subtype="bot_message")
        records, _ = exporter.build_archive(self.plan())
        validate_records(records)
        self.assertEqual(len(records["users"]), 2)
        self.assertEqual(records["users"][1]["status"], "deactivated")

    def test_conflicting_bot_references_fail(self):
        for user in self.data["users.json"]:
            user.update(is_bot=True, profile={"bot_id": "B1"})
        with self.assertRaisesRegex(ApplyError, "Conflicting bot identity"):
            exporter.build_archive(self.plan())

    def test_finder_metadata_is_ignored_but_other_entries_fail(self):
        self.plan()
        for parent in (self.source, self.source / "general"):
            (parent / ".DS_Store").write_bytes(b"Finder metadata")
        records, _ = exporter.build_archive(self.plan())
        validate_records(records)
        (self.source / "unknown.bin").write_bytes(b"Unknown")
        with self.assertRaisesRegex(ApplyError, "unsupported records"):
            exporter.build_archive(self.plan())

    def test_report_collects_independent_gaps_without_source_content(self):
        rows = self.data["general/2020-01-01.json"]
        rows[1]["reactions"] = [{"name": "private invalid name", "users": ["U2"], "count": 1}]
        rows[2]["blocks"] = [{"type": "unsupported_private_block"}]
        rows[2]["text"] = "private message body"
        report = exporter.conversion_report(self.plan())
        blockers = {item["code"]: item for item in report["blockers"]}
        self.assertEqual(blockers["unsupported_reaction_emoji"]["source_records"], [2])
        self.assertEqual(blockers["unsupported_message_content"]["source_records"], [3])
        self.assertEqual(report["unsupported_emoji_names"], 1)
        self.assertNotIn("private", json.dumps(report))
        self.assertNotIn("alice", json.dumps(report))

    def test_failed_conversion_writes_private_report_without_archive(self):
        self.data["general/2020-01-01.json"][1]["reactions"][0]["name"] = ":invalid:"
        self.plan()
        output = self.root / "output"
        report = self.root / "output.conversion-report.json"
        with patch.object(sys, "argv", ["slack-export.py", str(self.source), "--output", str(output)]), \
                patch.object(exporter, "FileCache") as cache, contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(exporter.main(), 1)
            cache.assert_not_called()
        data = json.loads(report.read_text())
        self.assertEqual(data["status"], "blocked")
        self.assertFalse(data["archive_written"])
        self.assertFalse(output.exists())
        self.assertEqual(report.stat().st_mode & 0o777, 0o600)

    def test_report_only_does_not_download_or_claim_archive_validation(self):
        self.with_file()
        self.plan()
        output = self.root / "output"
        report = self.root / "conversion-report.json"
        args = ["slack-export.py", str(self.source), "--output", str(output), "--report", str(report), "--report-only"]
        with patch.object(sys, "argv", args), patch.object(exporter, "FileCache") as cache:
            self.assertEqual(exporter.main(), 0)
            cache.assert_not_called()
        data = json.loads(report.read_text())
        self.assertEqual(data["status"], "scanned")
        self.assertEqual(data["file_checks"], "not_run")
        self.assertFalse(output.exists())
        before = report.read_bytes()
        with patch.object(sys, "argv", args), contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(exporter.main(), 1)
        self.assertEqual(report.read_bytes(), before)

    def test_source_read_failure_still_has_report(self):
        output = self.root / "output"
        with patch.object(sys, "argv", ["slack-export.py", str(self.source), "--output", str(output)]), \
                contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(exporter.main(), 1)
        report = json.loads((self.root / "output.conversion-report.json").read_text())
        self.assertEqual(report["blockers"][0]["code"], "source_read_failed")

    def test_report_channel_metadata_and_membership_checks(self):
        rows = self.data["general/2020-01-01.json"]
        rows[-1].pop("user")  # Channel metadata permits an unknown actor.
        report = exporter.conversion_report(self.plan())
        self.assertEqual(report["blockers"], [])
        rows[-1].pop("topic")
        rows.append(dict(rows[0], ts="1577836806.000000"))
        report = exporter.conversion_report(self.plan())
        self.assertEqual({b["code"] for b in report["blockers"]},
                         {"channel_metadata_missing", "repeated_membership_transition"})

    def test_report_known_file_losses_follow_selected_policy(self):
        self.data["general/2020-01-01.json"][1]["files"] = [{"id": "F1", "mode": "hidden_by_limit"}]
        for allow, section in ((False, "blockers"), (True, "omissions")):
            report = exporter.conversion_report(self.plan(), allow_missing_files=allow)
            self.assertIn("files_hidden_by_limit", {r["code"] for r in report[section]})

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
        report = json.loads((progress / "archive.conversion-report.json").read_text())
        self.assertEqual(report["status"], "complete")
        self.assertTrue(report["archive_written"])
        self.assertEqual(report["archive_omissions"], json.loads((output / "manifest.json").read_text())["omissions"])
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

    def test_unknown_custom_reaction_is_preserved(self):
        self.data["general/2020-01-01.json"][1]["reactions"][0]["name"] = "custom_company_emoji"
        plan = self.plan()
        records, _ = exporter.build_archive(plan)
        validate_records(records)
        reactions = [r for event in records["events"] for r in event["data"].get("reactions", [])]
        self.assertIn({"emoji": "custom_company_emoji", "user_ids": ["U2"]}, reactions)
        self.assertEqual(exporter.conversion_report(plan)["unsupported_emoji_names"], 0)

    def test_skin_tone_reactions_use_complete_shared_sequences(self):
        rows = self.data["general/2020-01-01.json"]
        rows[1]["reactions"] = [
            {"name": name, "users": ["U2"], "count": 1} for name in (
                "thumbsup::skin-tone-4", "woman_technologist::skin-tone-4",
                "handshake::skin-tone-2-6")]
        plan = self.plan()
        records, _ = exporter.build_archive(plan)
        validate_records(records)
        self.assertEqual({r["emoji"] for r in records["events"][1]["data"]["reactions"]},
                         {"👍🏽", "👩🏽‍💻", "🫱🏻‍🫲🏿"})
        self.assertEqual(exporter.conversion_report(plan)["unsupported_emoji_names"], 0)
        self.assertFalse(any("emoji images" in o["reason"] for o in records["manifest"]["omissions"]))

    def test_slack_aliases_preserve_tones_and_merge_equivalent_reactors(self):
        rows = self.data["general/2020-01-01.json"]
        rows[1]["reactions"] = [
            {"name": "the_horns::skin-tone-3", "users": ["U2"], "count": 1},
            {"name": "metal::skin-tone-3", "users": ["U1", "U2"], "count": 2},
            {"name": "male_elf::skin-tone-3", "users": ["U2"], "count": 1},
            {"name": "female-judge::skin-tone-3", "users": ["U2"], "count": 1},
        ]
        plan = self.plan()
        records, _ = exporter.build_archive(plan)
        validate_records(records)
        reactions = {r["emoji"]: r["user_ids"] for r in records["events"][1]["data"]["reactions"]}
        self.assertEqual(reactions, {"🤘🏼": ["U1", "U2"], "🧝🏼‍♂️": ["U2"], "👩🏼‍⚖️": ["U2"]})
        self.assertEqual(exporter.conversion_report(plan)["unsupported_emoji_names"], 0)

    def test_newer_standard_emoji_is_not_mislabeled_as_custom(self):
        aliases, supported = exporter.slack_emoji_catalog()
        with patch.object(exporter, "slack_emoji_catalog",
                          return_value=({**aliases, "future_emoji": "🫩"}, supported - {"🫩"})):
            self.assertEqual(exporter.reaction_unicode("future_emoji", {}), "future_emoji")
            self.data["general/2020-01-01.json"][1]["reactions"][0]["name"] = "future_emoji"
            plan = self.plan()
            records, _ = exporter.build_archive(plan)
            validate_records(records)
            omissions = records["manifest"]["omissions"]
            self.assertTrue(any(o["reason"].startswith("Newer standard reaction") for o in omissions))
            self.assertFalse(any(o["reason"].startswith("Custom reaction") for o in omissions))
            report = exporter.conversion_report(plan)
            self.assertEqual(report["unsupported_emoji_names"], 0)
            self.assertIn("standard_reaction_name_without_destination_glyph",
                          {item["code"] for item in report["transformations"]})
        with patch.object(exporter, "slack_emoji_catalog", return_value=({"future::skin-tone-2": "🫩🏻"}, frozenset())):
            with self.assertRaisesRegex(ApplyError, "destination catalog"):
                exporter.reaction_unicode("future::skin-tone-2", {})

    def test_supplemental_standard_emoji_convert_to_unicode(self):
        emojis = json.loads(Path(__file__).with_name("emoji-map.json").read_text())
        for name, glyph in {
            "face_with_bags_under_eyes": "🫩", "head_shaking_vertically": "🙂‍↕️",
            "leafless_tree": "🪾", "phoenix": "🐦‍🔥", "shovel": "🪏", "splatter": "🫟",
        }.items():
            with self.subTest(name=name):
                self.assertEqual(exporter.reaction_unicode(name, emojis), glyph)

    def test_invalid_skin_tone_combinations_are_not_constructed(self):
        emojis = json.loads(Path(__file__).with_name("emoji-map.json").read_text())
        for name in ("heart::skin-tone-4", "custom::skin-tone-4", "hugs::skin-tone-4",
                     "thumbsup::skin-tone-1", "thumbsup::skin-tone-7", "thumbsup::skin-tone-4-4"):
            with self.subTest(name=name), self.assertRaises(ApplyError):
                exporter.reaction_unicode(name, emojis)

    def test_custom_reaction_images_are_declared_once_per_retained_name(self):
        reason = "Custom reaction names are retained; their emoji images are not included."
        records, _ = exporter.build_archive(self.plan())
        self.assertFalse(any(o["reason"] == reason for o in records["manifest"]["omissions"]))
        rows = self.data["general/2020-01-01.json"]
        rows[1]["reactions"] = [
            {"name": "custom_one", "users": ["U1", "U2"], "count": 2},
            {"name": "heart", "users": ["U2"], "count": 1},
        ]
        rows[2]["reactions"] = [
            {"name": "custom_one", "users": ["U2"], "count": 1},
            {"name": "custom_two", "users": ["U1"], "count": 1},
            {"name": "custom_without_reactors", "users": [], "count": 1},
        ]
        records, _ = exporter.build_archive(self.plan())
        validate_records(records)
        omissions = [o for o in records["manifest"]["omissions"] if o["reason"] == reason]
        self.assertEqual(omissions, [{"category": "unsupported", "reason": reason, "count": 2}])
        self.assertNotIn("custom_one", json.dumps(omissions))

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

    def test_channel_event_reactions_are_reported_separately_from_content(self):
        row = self.data["general/2020-01-01.json"][-1]
        row["reactions"] = [{"name": "heart", "users": ["U2"], "count": 1}]
        plan = self.plan()
        blockers = {item["code"]: item for item in exporter.conversion_report(plan)["blockers"]}
        self.assertEqual(blockers["channel_event_reactions"]["count"], 1)
        self.assertEqual(blockers["channel_event_reactions"]["source_records"], [6])
        self.assertNotIn("channel_event_content", blockers)
        with self.assertRaisesRegex(ApplyError, "channel event type"):
            exporter.build_archive(plan)
        row["blocks"] = [{"type": "unsupported"}]
        blockers = {item["code"] for item in exporter.conversion_report(self.plan())["blockers"]}
        self.assertIn("channel_event_reactions", blockers)
        self.assertIn("channel_event_content", blockers)

    def test_membership_reactions_preserve_event_identity_and_timestamp(self):
        for index in (0, 4):
            self.data["general/2020-01-01.json"][index]["reactions"] = [
                {"name": "wave::skin-tone-3", "users": ["U1", "U2"], "count": 2},
                {"name": "custom_emoji", "users": ["U2"], "count": 1}]
        plan = self.plan()
        records, _ = exporter.build_archive(plan)
        validate_records(records)
        self.assertEqual(exporter.conversion_report(plan)["blockers"], [])
        for index, kind in ((0, "room.member_joined"), (4, "room.member_left")):
            event = records["events"][index]
            self.assertEqual(event["type"], kind)
            self.assertEqual(event["occurred_at"], f"2020-01-01T00:00:0{index}.000000Z")
            self.assertEqual(event["data"]["reactions"], [
                {"emoji": "custom_emoji", "user_ids": ["U2"]},
                {"emoji": "👋🏼", "user_ids": ["U1", "U2"]}])

    def test_private_channel_is_not_made_public(self):
        self.data["channels.json"][0]["is_private"] = True
        records, _ = exporter.build_archive(self.plan())
        self.assertEqual(records["rooms"][0]["visibility"], "private")

    def test_oversized_body_replacement_preserves_message_and_reports_loss(self):
        row = self.data["general/2020-01-01.json"][1]
        row["text"] = "x" * 10001
        row["files"] = [{"id": "F1", "mode": "hidden_by_limit"}]
        plan = self.plan()
        with self.assertRaises(exporter.OversizedBodyError):
            exporter.build_archive(plan, allow_missing_files=True)
        report = exporter.conversion_report(plan, allow_missing_files=True)
        self.assertIn("oversized_message_body", {b["code"] for b in report["blockers"]})
        records, _ = exporter.build_archive(plan, allow_missing_files=True, replace_oversized_messages=True)
        validate_records(records)
        root = records["events"][1]["data"]
        self.assertEqual(root["content"]["body"]["text"], exporter.OVERSIZED_BODY_NOTICE)
        self.assertEqual(root["content"]["asset_ids"], ["F1"])
        self.assertEqual(root["author_id"], "U1")
        self.assertEqual(root["created_at"], "2020-01-01T00:00:01.000001Z")
        self.assertEqual(root["edited_at"], "2020-01-01T00:00:10.000000Z")
        self.assertEqual(root["reactions"], [{"emoji": "❤️", "user_ids": ["U2"]}])
        self.assertEqual(records["events"][2]["data"]["thread_id"], root["message_id"])
        self.assertTrue(any(o["reason"].startswith("Oversized message bodies") and o["count"] == 1
                            for o in records["manifest"]["omissions"]))
        report = exporter.conversion_report(plan, allow_missing_files=True, replace_oversized_messages=True)
        self.assertEqual(report["blockers"], [])
        omission = next(o for o in report["omissions"] if o["code"] == "oversized_message_body")
        self.assertEqual(omission["source_records"], [2])

    def test_body_replacement_does_not_hide_other_conversion_errors(self):
        for value in ("", "*unsupported legacy formatting*"):
            self.data["general/2020-01-01.json"][1]["text"] = value
            with self.subTest(value=value), self.assertRaises(ApplyError):
                exporter.build_archive(self.plan(), replace_oversized_messages=True)

    def test_oversized_replacement_does_not_claim_attachment_text_was_retained(self):
        row = self.data["general/2020-01-01.json"][1]
        row["text"] = "x" * 10001
        row["attachments"] = [{"text": "Attachment summary"}]
        records, _ = exporter.build_archive(
            self.plan(), allow_attachment_summaries=True, replace_oversized_messages=True)
        validate_records(records)
        self.assertEqual(records["events"][1]["data"]["content"]["body"]["text"],
                         exporter.OVERSIZED_BODY_NOTICE)
        self.assertFalse(any(o["reason"].startswith("Attachment text is retained")
                             for o in records["manifest"]["omissions"]))
        report = exporter.conversion_report(
            self.plan(), allow_attachment_summaries=True, replace_oversized_messages=True)
        self.assertNotIn("attachment_summaries", {item["code"] for item in report["transformations"]})

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
        self.assertNotIn("source_bot_id", legacy["users"][0])


if __name__ == "__main__":
    unittest.main()
