"""Synthetic apply tests, including a committed write with a lost response."""

import copy
from pathlib import Path
import tempfile
import unittest
from unittest.mock import MagicMock, patch

from slack_apply import APIError, ApplyError, Journal, SQLiteJournal, Operator, MESSAGE_RETRY_SECONDS, apply, convert, fingerprint, save


class FakeOperator:
    def __init__(self):
        self.users = []
        self.rooms = []
        self.messages = []
        self.writes = []
        self.lose_message_response = False
        self.lose_create_response = None
        self.fail_before_create = False
        self.message_results = {}
        self.supports_idempotency = True
        self.supports_deleted = True
        self.emails = {}
        self.lose_email_response = False

    def call(self, service, method, request):
        if method == "ImportMessage" and request.get("deleted") and request.get("body"):
            if self.supports_deleted:
                raise APIError("invalid_argument", invalid_deleted_content=True)
            raise APIError("not_found")
        if method == "ImportMessage" and request.get("idempotencyKey") == "invalid":
            if self.supports_idempotency:
                raise APIError("invalid_argument", invalid_idempotency_key=True)
            raise APIError("not_found")
        if method == "ListUsers":
            return {"users": [{"user": u, "verifiedEmails": self.emails.get(u["id"], [])} for u in self.users]}
        if method == "ListRooms":
            return {"rooms": self.rooms}
        if method == "ImportMessage" and not request.get("roomId"):
            raise APIError("invalid_argument")
        if method == "ImportReaction" and not request:
            raise APIError("invalid_argument")
        if request.get("userId") == "slack-import-probe":
            raise APIError("not_found")
        if self.fail_before_create and method == "CreateUser":
            raise OSError("simulated failure before dispatch")
        self.writes.append((method, copy.deepcopy(request)))
        if method == "AddVerifiedEmail":
            emails = self.emails.setdefault(request["userId"], [])
            if request["email"] not in emails:
                emails.append(request["email"])
            if self.lose_email_response:
                raise OSError("simulated lost email response")
            return {}
        if method == "CreateUser":
            user = dict(request, id="user-" + str(len(self.users)))
            self.users.append(user)
            if self.lose_create_response == method:
                raise OSError("simulated lost response")
            return {"member": {"user": user}}
        if method == "CreateRoom":
            room = dict(request, id="room-" + str(len(self.rooms)))
            self.rooms.append(room)
            if self.lose_create_response == method:
                raise OSError("simulated lost response")
            return {"room": room}
        if method == "ImportMessage":
            key = request.get("idempotencyKey")
            if key in self.message_results:
                previous, response = self.message_results[key]
                if previous != request:
                    raise APIError("invalid_argument")
                if self.lose_message_response:
                    raise OSError("simulated lost retry response")
                return response
            self.messages.append(request)
            response = {"messageId": "message-" + str(len(self.messages))}
            self.message_results[key] = (copy.deepcopy(request), response)
            if self.lose_message_response:
                raise OSError("simulated lost response")
            return response
        return {}


class ApplyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name) / "journal"
        self.api = FakeOperator()
        self.plan = {
            "summary": {"issues": {}, "reactions": 1, "files": 0}, "canvases": [],
            "users": [{"source_id": "U1", "target_id": None, "proposed_login": "one",
                       "proposed_display_name": "One", "deactivated": False, "bot": False}],
            "rooms": [{"source_id": "C1", "target_id": None, "name": "general", "topic": "Topic",
                       "purpose": "Purpose", "members": ["U1"], "archived": False}],
            "messages": [{"source_key": "C1:1", "source_channel_id": "C1", "source_author_id": "U1",
                          "created_at": "2025-01-01T00:00:00.000001Z", "edited_at": None,
                          "thread_root_key": "C1:1", "source": {"text": "Root"}},
                         {"source_key": "C1:2", "source_channel_id": "C1", "source_author_id": "U1",
                          "created_at": "2025-01-01T00:00:00.000002Z", "edited_at": None,
                          "thread_root_key": "C1:1", "source": {"text": "Reply", "reactions": [
                              {"name": "hugging_face", "users": ["U1"]}]}}]}

    def journal(self):
        journal = Journal(self.directory, {"test": True})
        self.addCleanup(journal.close)
        return journal

    def test_apply_and_resume_preserve_threads_without_repeating_writes(self):
        journal = self.journal()
        result = apply(self.plan, self.api, journal)
        self.assertEqual(result["messages"], 2)
        self.assertTrue(self.api.messages[0]["createThread"])
        self.assertEqual(self.api.messages[1]["threadRootEventId"], "message-1")
        self.assertEqual(self.api.messages[1]["createdAt"], "2025-01-01T00:00:00.000002Z")
        reaction = self.api.writes[-1][1]
        self.assertEqual((reaction["userId"], reaction["emoji"]), ("user-0", "hugs"))
        count = len(self.api.writes)
        journal.close()
        apply(self.plan, self.api, self.journal())
        self.assertEqual(len(self.api.writes), count)
        self.assertEqual(journal.path.stat().st_mode & 0o777, 0o600)

    def test_email_assignment_recovers_after_lost_response(self):
        self.plan["users"][0]["verified_email"] = "one@example.org"
        self.plan["slack_workspace_ids"] = ["T1"]
        self.api.lose_email_response = True
        journal = self.journal()
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        self.assertEqual(journal.data["pending"]["method"], "AddVerifiedEmail")
        self.assertEqual(self.api.messages, [])
        journal.close()
        self.api.lose_email_response = False
        result = apply(self.plan, self.api, self.journal())
        self.assertEqual(result["imported_emails"], 1)
        self.assertEqual(self.api.emails, {"user-0": ["one@example.org"]})
        self.assertEqual(len(self.api.users), 1)
        self.assertNotIn("password", self.api.users[0])

    def test_email_collision_stops_before_writes(self):
        self.plan["users"][0]["verified_email"] = "one@example.org"
        self.api.users = [{"id": "bootstrap", "login": "admin"}]
        self.api.emails["bootstrap"] = ["ONE@example.org"]
        with self.assertRaisesRegex(ApplyError, "email conflicts"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.writes, [])

    def test_removed_imported_email_stops_resume(self):
        self.plan["users"][0]["verified_email"] = "one@example.org"
        journal = self.journal()
        apply(self.plan, self.api, journal)
        self.api.emails.clear()
        count = len(self.api.writes)
        with self.assertRaisesRegex(ApplyError, "email was removed"):
            apply(self.plan, self.api, journal)
        self.assertEqual(len(self.api.writes), count)

    def test_legacy_unkeyed_lost_response_still_blocks_resume(self):
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        self.assertEqual(len(self.api.messages), 1)
        del journal.data["pending"]["request"]["idempotencyKey"]
        journal.persist()
        journal.close()
        with self.assertRaisesRegex(ApplyError, "Uncertain message"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(len(self.api.messages), 1)

    def test_lost_response_replays_exact_request_after_restart(self):
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        pending = copy.deepcopy(journal.data["pending"])
        journal.close()
        self.api.lose_message_response = False
        apply(self.plan, self.api, self.journal())
        requests = [r for method, r in self.api.writes if method == "ImportMessage"]
        self.assertEqual(requests[0], requests[1])
        self.assertEqual(requests[1], pending["request"])
        self.assertNotEqual(requests[1]["idempotencyKey"], requests[2]["idempotencyKey"])
        self.assertEqual(len(self.api.messages), 2)

    def test_expired_or_backward_clock_blocks_replay_without_writes(self):
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        first = journal.data["pending"]["first_attempt_at"]
        count = len(self.api.writes)
        for now in (first + MESSAGE_RETRY_SECONDS, first - 1):
            with patch("slack_apply.time.time", return_value=now):
                with self.assertRaisesRegex(ApplyError, "safe retry window"):
                    apply(self.plan, self.api, journal)
        self.assertEqual(len(self.api.writes), count)

    def test_repeated_lost_response_does_not_extend_window(self):
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        pending = copy.deepcopy(journal.data["pending"])
        with patch("slack_apply.time.time", return_value=pending["first_attempt_at"] + 60):
            with self.assertRaises(OSError):
                apply(self.plan, self.api, journal)
        self.assertEqual(journal.data["pending"], pending)
        self.assertEqual(len(self.api.messages), 1)

    def test_destination_changes_block_pending_message_replay(self):
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        count = len(self.api.writes)
        self.api.users.append({"id": "other", "login": "other"})
        with self.assertRaisesRegex(ApplyError, "outside this import"):
            apply(self.plan, self.api, journal)
        self.assertEqual(len(self.api.writes), count)

    def test_old_server_rejected_before_writes(self):
        self.api.supports_idempotency = False
        with self.assertRaisesRegex(ApplyError, "idempotent historical imports"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.writes, [])

    def test_source_error_causes_zero_writes(self):
        self.plan["messages"][-1]["source"]["blocks"] = [{"type": "image"}]
        with self.assertRaisesRegex(ApplyError, "Unsupported Slack block"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.writes, [])

    def test_recovered_response_resumes_without_duplicate_message(self):
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        digest = fingerprint(journal.data["pending"])
        with self.assertRaises(ApplyError):
            journal.reconcile({"pending_sha256": "other", "response": {"messageId": "message-1"}})
        with self.assertRaises(ApplyError):
            journal.reconcile({"pending_sha256": digest, "response": {}})
        journal.reconcile({"pending_sha256": digest, "response": {"messageId": "message-1"}})
        self.api.lose_message_response = False
        apply(self.plan, self.api, journal)
        self.assertEqual(len(self.api.messages), 2)
        self.assertEqual(len(journal.data["reconciliations"]), 1)

    def test_existing_channel_rejected_before_writes(self):
        self.api.rooms = [{"id": "existing", "name": "unrelated"}]
        with self.assertRaisesRegex(ApplyError, "blank instance"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.writes, [])

    def test_lost_user_or_room_response_recovers_automatically(self):
        for method in ("CreateUser", "CreateRoom"):
            with self.subTest(method=method):
                self.directory = Path(self.temp.name) / method
                self.api = FakeOperator()
                self.api.lose_create_response = method
                journal = self.journal()
                with self.assertRaises(OSError):
                    apply(self.plan, self.api, journal)
                journal.close()
                self.api.lose_create_response = None
                apply(self.plan, self.api, self.journal())
                self.assertEqual(len(self.api.users), 1)
                self.assertEqual(len(self.api.rooms), 1)
                self.assertEqual(len(self.api.messages), 2)

    def test_bootstrap_state_is_retained_and_replacement_rejected(self):
        self.plan["rooms"][0]["name"] = "source-general"
        self.api.users = [{"id": "bootstrap", "login": "admin"}]
        self.api.rooms = [{"id": "bootstrap-room", "name": "general", "universal": True}]
        journal = self.journal()
        apply(self.plan, self.api, journal)
        self.assertEqual(self.api.users[0]["login"], "admin")
        self.assertEqual(self.api.rooms[0]["name"], "general")
        with self.assertRaisesRegex(ApplyError, "bootstrap state changed"):
            apply(self.plan, FakeOperator(), journal)

    def test_pending_create_without_server_write_retries(self):
        journal = self.journal()
        self.api.fail_before_create = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        self.assertEqual(self.api.users, [])
        journal.close()
        self.api.fail_before_create = False
        apply(self.plan, self.api, self.journal())
        self.assertEqual(len(self.api.users), 1)
        self.assertEqual(len(self.api.messages), 2)

    def test_existing_login_without_pending_create_is_not_adopted(self):
        journal = self.journal()
        self.api.fail_before_create = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        login = journal.data["pending"]["request"]["login"]
        journal.close()
        self.directory = Path(self.temp.name) / "other-journal"
        self.api = FakeOperator()
        self.api.users = [{"id": "unrelated", "login": login}]
        with self.assertRaisesRegex(ApplyError, "bootstrap login"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.writes, [])

    def test_journal_rejects_other_destination_and_concurrent_run(self):
        journal = self.journal()
        with self.assertRaises(ApplyError):
            Journal(self.directory, {"test": True})
        journal.close()
        with self.assertRaises(ApplyError):
            Journal(self.directory, {"destination": "other"})

    def test_rich_text_mentions_formatting_and_missing_files(self):
        source = {"blocks": [{"type": "rich_text", "elements": [{"type": "rich_text_section", "elements": [
            {"type": "text", "text": "Hi ", "style": {"bold": True}},
            {"type": "user", "user_id": "U1"},
            {"type": "text", "text": " *literal*"}]}]}], "files": [{"mode": "hidden_by_limit"}]}
        with self.assertRaises(ApplyError):
            convert(source, {"U1": "mapped"}, False)
        self.assertEqual(convert(source, {"U1": "mapped"}, True),
                         "**Hi** @mapped &#42;literal&#42;\n\n[Attachment unavailable in this import]")
        with self.assertRaisesRegex(ApplyError, "Unmapped mention"):
            convert(source, {}, True)

    def test_attachment_summary_requires_opt_in_and_never_embeds_html(self):
        source = {"text": "Link", "attachments": [{"title": "Example", "text": "*literal* &amp; safe",
                  "fallback": "Example", "title_link": "https://example.org/page",
                  "video_html": '<iframe src="https://untrusted.invalid"></iframe>',
                  "image_url": "https://untrusted.invalid/image",
                  "fields": [{"title": "Status", "value": "Ready"}]}]}
        with self.assertRaisesRegex(ApplyError, "allow-attachment-summaries"):
            convert(source, {}, False)
        result = convert(source, {}, False, True)
        self.assertNotIn("Slack attachment summary", result)
        self.assertIn("[Source](<https://example.org/page>)", result)
        self.assertIn("&#42;literal&#42; &#38; safe", result)
        self.assertIn("Status — Ready", result)
        self.assertEqual(result.count("Example"), 1)
        self.assertNotIn("iframe", result)
        self.assertNotIn("untrusted", result)

    def test_unsafe_or_unrepresentable_attachments_still_fail(self):
        for attachment in ({"fallback": "button", "actions": [{}]}, {"blocks": [{}]},
                           {"text": "link", "title_link": "javascript:alert(1)"}):
            with self.assertRaises(ApplyError):
                convert({"attachments": [attachment]}, {}, True, True)
        source = {"attachments": [{"fallback": "file", "files": [{"id": "F1"}]}]}
        with self.assertRaisesRegex(ApplyError, "allow-missing-files"):
            convert(source, {}, False, True)
        self.assertIn("Attachment unavailable", convert(source, {}, True, True))

    def test_channel_notices_actions_and_broadcasts(self):
        notice = convert({"subtype": "channel_join", "text": "<@U1> joined &amp; waved"}, {}, False)
        self.assertIn("Slack channel notice", notice)
        self.assertIn("&#60;@U1&#62;", notice)
        self.assertEqual(convert({"subtype": "me_message", "text": "waves"}, {}, False),
                         "[Slack action]\n\nwaves")
        broadcast = {"subtype": "thread_broadcast", "text": "reply", "ts": "2.000000", "thread_ts": "1.000000"}
        self.assertEqual(convert(broadcast, {}, False), "reply")
        broadcast.pop("thread_ts")
        with self.assertRaisesRegex(ApplyError, "thread root"):
            convert(broadcast, {}, False)
        for subtype in ("message_deleted", "unknown"):
            with self.assertRaisesRegex(ApplyError, "Unsupported message subtype"):
                convert({"subtype": subtype, "text": "not a normal message"}, {}, False)

    def test_attachment_policy_reaches_import_before_mutations(self):
        self.plan["messages"][0]["source"]["attachments"] = [{"title": "Example", "text": "Preview"}]
        self.plan["summary"]["issues"]["slack_attachment_conversion_required"] = 1
        journal = self.journal()
        with self.assertRaisesRegex(ApplyError, "allow-attachment-summaries"):
            apply(self.plan, self.api, journal)
        self.assertEqual(self.api.writes, [])
        apply(self.plan, self.api, journal, allow_attachment_summaries=True)
        self.assertIn("Example\n\nPreview", self.api.messages[0]["body"])

    def test_shared_message_has_one_quote_and_no_fallback_metadata(self):
        source = {"attachments": [{"is_msg_unfurl": True, "author_name": "Example Person",
                  "text": "A question?\nSecond line", "fallback": "[date] user: A question?",
                  "footer": "Slack conversation", "author_link": "https://example.slack.com/team/U1",
                  "original_url": "https://example.slack.com/archives/C1/p123?x=a&y=b"}]}
        result = convert(source, {}, False, True)
        self.assertIn("**Example Person**\n\n> A question?\n> Second line", result)
        self.assertEqual(result.count("A question?"), 1)
        self.assertIn("[Original Slack message](<https://example.slack.com/archives/C1/p123?x=a&amp;y=b>)", result)
        self.assertNotIn("[date]", result)
        self.assertNotIn("/team/", result)
        self.assertNotIn("Slack conversation", result)

    def test_attachment_fallback_only_when_primary_content_is_absent(self):
        for shared in (False, True):
            source = {"attachments": [{"is_msg_unfurl": shared, "fallback": "Fallback only"}]}
            self.assertIn("Fallback only", convert(source, {}, False, True))
            source["attachments"][0]["text"] = "Primary content"
            result = convert(source, {}, False, True)
            self.assertIn("Primary content", result)
            self.assertNotIn("Fallback only", result)

    def test_thread_broadcast_imports_once_in_original_thread(self):
        self.plan["messages"][1]["source"]["subtype"] = "thread_broadcast"
        self.plan["messages"][1]["source"]["ts"] = "2.000000"
        self.plan["messages"][1]["source"]["thread_ts"] = "1.000000"
        result = apply(self.plan, self.api, self.journal())
        self.assertEqual(len(self.api.messages), 2)
        self.assertEqual(self.api.messages[1]["threadRootEventId"], "message-1")
        self.assertEqual(result["broadcasts_without_channel_echo"], 1)

    def test_attachment_text_cannot_bypass_message_size_limit(self):
        with self.assertRaisesRegex(ApplyError, "body limit"):
            convert({"attachments": [{"text": "x" * 10001}]}, {}, True, True)

    def test_tombstone_recovers_without_content_and_keeps_replies(self):
        root = self.plan["messages"][0]
        root["source"]["subtype"] = "tombstone"
        root["source"]["text"] = "Deleted source placeholder must never be sent"
        root["source"]["reactions"] = [{"name": "heart", "count": 1, "users": ["U1"]}]
        root["edited_at"] = "2025-01-01T00:00:00Z"
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        self.assertTrue(self.api.messages[0]["deleted"])
        self.assertEqual(self.api.messages[0]["body"], "")
        self.assertNotIn("editedAt", self.api.messages[0])
        journal.close()
        self.api.lose_message_response = False
        result = apply(self.plan, self.api, self.journal())
        self.assertEqual(len(self.api.messages), 2)
        self.assertEqual(self.api.messages[1]["threadRootEventId"], "message-1")
        self.assertEqual(result["tombstones"], 1)
        reactions = [request for method, request in self.api.writes if method == "ImportReaction"
                     and request.get("messageEventId") == "message-1"]
        self.assertEqual(len(reactions), 1)
        self.assertEqual(reactions[0]["emoji"], "heart")

    def test_old_server_rejects_tombstones_before_writes(self):
        self.plan["messages"][0]["source"]["subtype"] = "tombstone"
        self.api.supports_deleted = False
        with self.assertRaisesRegex(ApplyError, "deleted historical imports"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.writes, [])

    def test_tombstone_does_not_silently_discard_other_content(self):
        for field in ("files", "attachments", "blocks"):
            with self.assertRaisesRegex(ApplyError, "Tombstone contains"):
                convert({"subtype": "tombstone", field: [{}]}, {}, True, True)


class SQLiteApplyTests(ApplyTests):
    """Run the mutation and lost-response cases against the durable database."""

    def journal(self):
        journal = SQLiteJournal(self.directory, {"test": True})
        self.addCleanup(journal.close)
        return journal


class NamingTests(unittest.TestCase):
    setUp = ApplyTests.setUp
    journal = ApplyTests.journal

    def test_source_names_and_periods_are_preserved(self):
        self.plan["users"][0]["proposed_login"] = "First.Last"
        apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.users[0]["login"], "First.Last")
        self.assertEqual(self.api.rooms[0]["name"], "general")

    def test_optional_channel_prefix(self):
        apply(self.plan, self.api, self.journal(), channel_prefix="old-")
        self.assertEqual(self.api.rooms[0]["name"], "old-general")

    def test_renamed_bootstrap_universal_channel_is_retained(self):
        self.api.rooms = [{"id": "bootstrap", "name": "chatto-general", "universal": True}]
        apply(self.plan, self.api, self.journal())
        self.assertEqual([room["name"] for room in self.api.rooms], ["chatto-general", "general"])

    def test_bootstrap_collision_stops_before_writes(self):
        self.api.rooms = [{"id": "bootstrap", "name": "general", "universal": True}]
        with self.assertRaisesRegex(ApplyError, "room name conflicts"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.writes, [])

    def test_invalid_username_is_not_silently_changed(self):
        self.plan["users"][0]["proposed_login"] = "invalid name"
        with self.assertRaisesRegex(ApplyError, "no automatic rename"):
            apply(self.plan, self.api, self.journal())
        self.assertEqual(self.api.writes, [])


class OperatorTransportTests(unittest.TestCase):
    def test_reuses_connection_without_retrying_uncertain_write(self):
        with patch("slack_apply.http.client.HTTPConnection") as factory, patch("slack_apply.socket.socket"):
            connection = factory.return_value
            response = MagicMock(status=200, will_close=False)
            response.read.return_value = b'{}'
            connection.getresponse.return_value = response
            api = Operator("/synthetic/socket")
            api.call("Room", "ListRooms", {})
            api.call("Room", "ListRooms", {})
            self.assertEqual(factory.call_count, 1)
            connection.getresponse.side_effect = OSError("lost response")
            with self.assertRaises(OSError):
                api.call("Message", "ImportMessage", {})
            self.assertEqual(connection.request.call_count, 3)
            self.assertIsNone(api.connection)
            connection.getresponse.side_effect = None
            api.call("Room", "ListRooms", {})
            self.assertEqual(factory.call_count, 2)
            api.close()


class JournalMigrationTests(unittest.TestCase):
    def test_pending_message_survives_json_migration_and_reopen(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary) / "progress"
            api = FakeOperator()
            old = Journal(directory, {"source": "synthetic"})
            api.lose_message_response = True
            with self.assertRaises(OSError):
                old.write("message", "Message", "ImportMessage", {"roomId": "room", "body": "test"}, api)
            pending = copy.deepcopy(old.data["pending"])
            old.close()
            migrated = SQLiteJournal(directory, {"source": "synthetic"})
            self.assertEqual(migrated.data["pending"], pending)
            api.lose_message_response = False
            migrated.recover({}, {}, api)
            migrated.close()
            resumed = SQLiteJournal(directory, {"source": "synthetic"})
            self.assertIsNone(resumed.data["pending"])
            self.assertIn("message", resumed.data["done"])
            self.assertEqual(len(api.messages), 1)
            resumed.close()


if __name__ == "__main__":
    unittest.main()
