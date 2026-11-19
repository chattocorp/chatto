"""Read Slack account emails without logging credentials or API payloads.

Only auth.test and users.list are contacted. Redirects are refused so bearer
credentials cannot leave Slack's API host. Identity enrichment is opt-in.
"""

import json
import re
import stat
import time
import urllib.error
import urllib.parse
import urllib.request

from slack_apply import ApplyError


def read_token(path=None, stream=None):
    """Read a bounded credential from a private regular file or stdin."""
    try:
        if path is not None:
            if path.is_symlink() or not stat.S_ISREG(path.stat().st_mode) or path.stat().st_mode & 0o077:
                raise ApplyError("Slack token file must be a regular, non-symlink file with mode 0600.")
            with path.open() as source:
                token = source.read(8193)
        else:
            token = stream.read(8193)
    except (OSError, UnicodeError):
        raise ApplyError("Cannot read Slack token.") from None
    if len(token) > 8192:
        raise ApplyError("Slack token exceeds the size limit.")
    token = token.strip()
    if not token or not token.isascii() or any(c.isspace() for c in token):
        raise ApplyError("Slack token is empty or malformed.")
    return token


class NoRedirect(urllib.request.HTTPRedirectHandler):
    """Never forward an authenticated request to a redirect destination."""

    def redirect_request(self, *args, **kwargs):
        return None


class SlackAPI:
    """Read-only, paginated Slack client with bounded retries and safe errors."""

    def __init__(self, token):
        self._token = token
        self._opener = urllib.request.build_opener(NoRedirect)
        self.scopes = set()

    def call(self, method, parameters=None):
        if method not in {"auth.test", "users.list"}:
            raise ApplyError("Unsupported Slack API method.")
        # Slack users.list expects query/form parameters, not a JSON request body.
        url = "https://slack.com/api/" + method + "?" + urllib.parse.urlencode(parameters or {})
        request = urllib.request.Request(url, headers={"Authorization": "Bearer " + self._token})
        for attempt in range(4):
            try:
                with self._opener.open(request, timeout=30) as response:
                    raw = response.read(8 * 1024 * 1024 + 1)
                    if len(raw) > 8 * 1024 * 1024:
                        raise ApplyError("Slack API response exceeds the size limit.")
                    result = json.loads(raw)
                    self.scopes.update(s.strip() for s in response.headers.get("x-oauth-scopes", "").split(","))
            except urllib.error.HTTPError as error:
                code, retry_after = error.code, error.headers.get("Retry-After", "")
                error.close()
                if attempt < 3 and (code == 429 or 500 <= code <= 599):
                    delay = int(retry_after) if retry_after.isdigit() else 2 ** attempt
                    if delay > 60:
                        raise ApplyError("Slack rate limit requires a longer wait; retry the command later.") from None
                    time.sleep(max(1, delay))
                    continue
                raise ApplyError("Slack HTTP request failed; check token permissions or retry later.") from None
            except (OSError, ValueError, UnicodeError):
                raise ApplyError("Slack network or JSON response error.") from None
            if not isinstance(result, dict) or result.get("ok") is not True:
                if isinstance(result, dict) and result.get("error") == "missing_scope":
                    raise ApplyError("Slack token requires users:read and users:read.email.")
                raise ApplyError("Slack rejected the request; check the token and its permissions.")
            return result

    def users(self):
        """Read each user once; reject partial or ambiguous directory responses."""
        users, seen, cursor = {}, set(), ""
        for _ in range(1000):
            page = self.call("users.list", {"limit": 200, **({"cursor": cursor} if cursor else {})})
            members = page.get("members")
            metadata = page.get("response_metadata", {})
            if not isinstance(members, list) or not isinstance(metadata, dict):
                raise ApplyError("Invalid Slack user directory response.")
            for user in members:
                key = user.get("id") if isinstance(user, dict) else None
                if not isinstance(key, str) or not key or key in users:
                    raise ApplyError("Missing or duplicate Slack API user ID.")
                users[key] = user
            cursor = metadata.get("next_cursor", "")
            if not isinstance(cursor, str):
                raise ApplyError("Invalid Slack pagination cursor.")
            if not cursor:
                return users
            if cursor in seen or not members:
                raise ApplyError("Slack pagination did not advance.")
            seen.add(cursor)
        raise ApplyError("Slack user directory exceeds the pagination limit.")


def enrich_users(plan, api):
    """Attach API-verified identity mappings; return aggregate counts only.

The export remains authoritative for membership and account status. Active
human accounts need a unique email. Conflicting export/API identity data fails
before destination writes. Only matched export users affect the resume binding.
"""
    workspace_ids = plan.get("slack_workspace_ids", [])
    if len(workspace_ids) != 1:
        raise ApplyError("Slack API enrichment requires an export identifying one workspace.")
    auth = api.call("auth.test")
    if auth.get("team_id") != workspace_ids[0]:
        raise ApplyError("Slack token belongs to a different workspace.")
    if not {"users:read", "users:read.email"} <= api.scopes:
        raise ApplyError("Slack token requires users:read and users:read.email.")
    directory = api.users()
    report = {"workspace_verified": True, "api_users": len(directory), "export_users": len(plan["users"]),
              "matched_users": 0, "active_humans": 0, "active_humans_with_email": 0}
    emails, updates = set(), []
    for user in plan["users"]:
        live = directory.get(user["source_id"])
        report["matched_users"] += live is not None
        if user["bot"] or user["deactivated"] or user["source_id"] == "USLACKBOT":
            continue
        report["active_humans"] += 1
        if live is None:
            raise ApplyError("An active exported user is missing from the Slack API directory.")
        if live.get("team_id") != workspace_ids[0]:
            raise ApplyError("Slack API user belongs to a different workspace.")
        if live.get("deleted") or live.get("is_bot") or live.get("is_app_user"):
            raise ApplyError("Slack account status changed since export; review or obtain a new export.")
        profile = live.get("profile") or {}
        email = profile.get("email") if isinstance(profile, dict) else None
        if (not isinstance(email, str) or len(email) > 320
                or not re.fullmatch(r"[^\s@<>\x00-\x1f\x7f]+@[^\s@<>\x00-\x1f\x7f]+\.[^\s@<>\x00-\x1f\x7f]+", email)):
            raise ApplyError("An active Slack user has no usable email address.")
        email = email.lower()
        exported_email = user.get("source_email")
        if exported_email and (not isinstance(exported_email, str) or exported_email.lower() != email):
            raise ApplyError("Slack API email differs from the export; review or obtain a new export.")
        if email in emails:
            raise ApplyError("Active Slack users have duplicate email addresses.")
        emails.add(email)
        updates.append((user, email))
        report["active_humans_with_email"] += 1
    for user, email in updates:
        user["verified_email"] = email
    return report
