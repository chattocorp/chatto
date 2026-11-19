"""Synthetic Slack transport, identity, and credential tests; no real tokens."""

import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch
import urllib.error
import urllib.parse

from slack_api import SlackAPI, NoRedirect, enrich_users, read_token
from slack_apply import ApplyError, Journal, fingerprint


def response(body, scopes="users:read,users:read.email"):
    result = io.BytesIO(json.dumps(body).encode())
    result.headers = {"x-oauth-scopes": scopes}
    return result


def directory_user(key="U1", email="one@example.org"):
    return {"id": key, "team_id": "T1", "profile": {"email": email}}


class SlackAPITests(unittest.TestCase):
    def setUp(self):
        self.api = SlackAPI("secret-token")
        self.api._opener = Mock()

    def test_query_pagination_and_authorization(self):
        self.api._opener.open.side_effect = [
            response({"ok": True, "members": [directory_user()], "response_metadata": {"next_cursor": "next/+="}}),
            response({"ok": True, "members": [directory_user("U2", "two@example.org")]})]
        self.assertEqual(set(self.api.users()), {"U1", "U2"})
        requests = [call.args[0] for call in self.api._opener.open.call_args_list]
        self.assertEqual(urllib.parse.parse_qs(urllib.parse.urlsplit(requests[1].full_url).query)["cursor"], ["next/+="])
        self.assertEqual(requests[0].get_header("Authorization"), "Bearer secret-token")
        self.assertNotIn("secret-token", requests[0].full_url)
        self.assertIsNone(requests[0].data)
        self.assertIn("users:read.email", self.api.scopes)

    def test_rate_limit_retry_and_bound(self):
        error = urllib.error.HTTPError("private-url", 429, "private-error", {"Retry-After": "4"}, io.BytesIO(b"private"))
        self.api._opener.open.side_effect = [error, response({"ok": True, "team_id": "T1"})]
        with patch("slack_api.time.sleep") as sleep:
            self.api.call("auth.test")
        sleep.assert_called_once_with(4)
        self.api._opener.open.side_effect = [urllib.error.HTTPError("private-url", 429, "private", {"Retry-After": "61"}, None)]
        with self.assertRaisesRegex(ApplyError, "longer wait"):
            self.api.call("auth.test")

    def test_error_payloads_never_escape(self):
        for payload in ({"ok": False, "error": "private-secret"}, {"ok": False, "error": "missing_scope"}, [], {}):
            self.api._opener.open.return_value = response(payload)
            with self.assertRaises(ApplyError) as caught:
                self.api.call("auth.test")
            self.assertNotIn("private-secret", str(caught.exception))
        self.api._opener.open.side_effect = OSError("secret-token private-email@example.org")
        with self.assertRaisesRegex(ApplyError, "network") as caught:
            self.api.call("auth.test")
        self.assertNotIn("secret-token", str(caught.exception))

    def test_redirects_and_writes_are_disallowed(self):
        self.assertIsNone(NoRedirect().redirect_request(None, None, 302, "", {}, "https://other.example"))
        with self.assertRaisesRegex(ApplyError, "Unsupported"):
            self.api.call("chat.postMessage")
        self.api._opener.open.assert_not_called()
        self.api._opener.open.side_effect = urllib.error.HTTPError("https://slack.com/api/auth.test", 302, "redirect", {}, None)
        with self.assertRaises(ApplyError):
            self.api.call("auth.test")
        self.assertEqual(self.api._opener.open.call_count, 1)

    def test_duplicate_users_and_nonadvancing_pagination_fail(self):
        for pages in ([{"ok": True, "members": [directory_user(), directory_user()]}],
                      [{"ok": True, "members": [directory_user()], "response_metadata": {"next_cursor": "same"}},
                       {"ok": True, "members": [directory_user("U2")], "response_metadata": {"next_cursor": "same"}}]):
            self.api._opener.open.side_effect = [response(p) for p in pages]
            with self.assertRaises(ApplyError):
                self.api.users()

    def test_token_file_permissions_stdin_and_symlinks(self):
        self.assertEqual(read_token(stream=io.StringIO("secret-token\n")), "secret-token")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "token"
            path.write_text("secret-token")
            path.chmod(0o644)
            with self.assertRaisesRegex(ApplyError, "0600"):
                read_token(path)
            path.chmod(0o600)
            self.assertEqual(read_token(path), "secret-token")
            link = path.with_name("link")
            link.symlink_to(path)
            with self.assertRaises(ApplyError):
                read_token(link)
        for value in ("", "secret token", "x" * 8193):
            with self.assertRaises(ApplyError):
                read_token(stream=io.StringIO(value))


class IdentityTests(unittest.TestCase):
    def setUp(self):
        self.plan = {"slack_workspace_ids": ["T1"], "users": [
            {"source_id": "U1", "source_email": None, "bot": False, "deactivated": False}]}
        self.api = Mock(scopes={"users:read", "users:read.email"})
        self.api.call.return_value = {"team_id": "T1"}
        self.api.users.return_value = {"U1": directory_user()}

    def test_match_by_id_not_name_and_report_only_counts(self):
        self.api.users.return_value["U2"] = directory_user("U2", "other@example.org")
        self.api.users.return_value["U1"]["name"] = "changed-login"
        report = enrich_users(self.plan, self.api)
        self.assertEqual(self.plan["users"][0]["verified_email"], "one@example.org")
        self.assertEqual(report["active_humans_with_email"], 1)
        self.assertNotIn("example.org", json.dumps(report))
        self.assertNotIn("U1", json.dumps(report))

    def test_workspace_and_scopes_fail_before_directory_read(self):
        for team, scopes in (("T2", {"users:read", "users:read.email"}), ("T1", {"users:read"})):
            self.api.call.return_value = {"team_id": team}
            self.api.scopes = scopes
            with self.assertRaises(ApplyError):
                enrich_users(self.plan, self.api)
            self.api.users.assert_not_called()

    def test_missing_email_status_conflict_and_export_conflict(self):
        for live in ({}, {"U1": directory_user(email="")}, {"U1": directory_user(email="bad")},
                     {"U1": dict(directory_user(), deleted=True)},
                     {"U1": dict(directory_user(), team_id="T2")}):
            self.api.users.return_value = live
            with self.assertRaises(ApplyError):
                enrich_users(self.plan, self.api)
            self.assertNotIn("verified_email", self.plan["users"][0])
        self.api.users.return_value = {"U1": directory_user()}
        self.plan["users"][0]["source_email"] = "different@example.org"
        with self.assertRaisesRegex(ApplyError, "differs"):
            enrich_users(self.plan, self.api)

    def test_duplicate_emails_rejected_before_plan_mutation(self):
        self.plan["users"].append(dict(self.plan["users"][0], source_id="U2"))
        self.api.users.return_value["U2"] = directory_user("U2", "ONE@example.org")
        with self.assertRaisesRegex(ApplyError, "duplicate email"):
            enrich_users(self.plan, self.api)
        self.assertTrue(all("verified_email" not in u for u in self.plan["users"]))

    def test_inactive_accounts_do_not_receive_login_email(self):
        self.plan["users"][0]["deactivated"] = True
        self.assertEqual(enrich_users(self.plan, self.api)["active_humans_with_email"], 0)
        self.assertNotIn("verified_email", self.plan["users"][0])

    def test_email_change_blocks_journal_resume_but_extra_api_users_do_not(self):
        enrich_users(self.plan, self.api)
        original = fingerprint(self.plan)
        self.api.users.return_value["U2"] = directory_user("U2", "extra@example.org")
        enrich_users(self.plan, self.api)
        self.assertEqual(fingerprint(self.plan), original)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "journal"
            Journal(path, {"source": original}).close()
            self.api.users.return_value["U1"] = directory_user(email="changed@example.org")
            enrich_users(self.plan, self.api)
            with self.assertRaisesRegex(ApplyError, "another archive"):
                Journal(path, {"source": fingerprint(self.plan)})


if __name__ == "__main__":
    unittest.main()
