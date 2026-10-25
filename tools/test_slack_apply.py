"""Synthetic apply tests, including a committed write with a lost response."""

import copy
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from slack_apply import APIError, ApplyError, Journal, MESSAGE_RETRY_SECONDS, apply, convert, fingerprint, save


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

    def call(self, service, method, request):
        if method == "ImportMessage" and request.get("idempotencyKey") == "invalid":
            if self.supports_idempotency:
                raise APIError("invalid_argument", invalid_idempotency_key=True)
            raise APIError("not_found")
        if method == "ListUsers":
            return {"users": [{"user": u} for u in self.users]}
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

    def test_legacy_unkeyed_lost_response_still_blocks_resume(self):
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(self.plan, self.api, journal)
        self.assertEqual(len(self.api.messages), 1)
        del journal.data["pending"]["request"]["idempotencyKey"]
        save(journal.path, journal.data)
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
                         "**Hi** @mapped \\*literal\\*\n\n[Attachment unavailable in this import]")
        with self.assertRaisesRegex(ApplyError, "Unmapped mention"):
            convert(source, {}, True)


if __name__ == "__main__":
    unittest.main()
