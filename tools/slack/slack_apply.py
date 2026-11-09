"""Import Slack records through Chatto's local operator Connect API.

Optional file downloads are managed by slack_files. A durable pending record precedes every message mutation;
uncertain message responses can be retried within the server's collision window.
"""

import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import socket
import sqlite3
import tempfile
import time
import unicodedata
import uuid
from urllib.parse import quote, urlsplit

# Stop before the server's fixed 30-minute window, allowing transport margin.
MESSAGE_RETRY_SECONDS = 29 * 60
INVALID_IDEMPOTENCY_KEY = "idempotency_key must be a non-zero UUID in hyphenated form"
INVALID_DELETED_CONTENT = "deleted imports must not contain body, attachments, preview, or edited_at"


class ApplyError(ValueError):
    """A safe, content-free diagnostic for the operator."""


class OversizedBodyError(ApplyError):
    """The converted UTF-8 body exceeds the supported message size."""


class APIError(ApplyError):
    def __init__(self, code, invalid_idempotency_key=False, invalid_deleted_content=False):
        super().__init__("Operator request failed (" + code + ").")
        self.code = code
        self.invalid_idempotency_key = invalid_idempotency_key
        self.invalid_deleted_content = invalid_deleted_content


class Operator:
    """JSON Connect transport restricted to a local Unix socket."""

    def __init__(self, path):
        self.path = str(Path(path).absolute())
        self.connection = None

    def call(self, service, method, body):
        try:
            if self.connection is None:
                self.connection = http.client.HTTPConnection("localhost", timeout=30)
                self.connection.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                self.connection.sock.settimeout(30)
                self.connection.sock.connect(self.path)
            connection = self.connection
            connection.request("POST", "/api/connect/chatto.operator.v1.Operator" + service + "Service/" + method,
                               json.dumps(body).encode(),
                               {"Content-Type": "application/json", "Connect-Protocol-Version": "1"})
            response = connection.getresponse()
            value = json.loads(response.read())
            if response.will_close:
                self.close()
            if response.status != 200:
                code = value.get("code")
                raise APIError(code if code in {"invalid_argument", "not_found", "already_exists", "unimplemented"} else "request_error",
                               value.get("message") == INVALID_IDEMPOTENCY_KEY,
                               value.get("message") == INVALID_DELETED_CONTENT)
            return value
        except Exception:
            # Never retry here: a lost response can follow a committed mutation.
            self.close()
            raise

    def close(self):
        if self.connection is not None:
            self.connection.close()
            self.connection = None


def save(path, value):
    """Atomically replace an owner-only journal and sync its directory."""
    descriptor, temporary = tempfile.mkstemp(dir=path.parent, prefix=".journal-")
    try:
        with os.fdopen(descriptor, "w") as output:
            json.dump(value, output, ensure_ascii=False, indent=2)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def fingerprint(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True).encode()).hexdigest()


class Journal:
    """Serialize runs and retain responses under stable source operation keys."""

    def __init__(self, directory, binding):
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        if directory.is_symlink() or directory.stat().st_mode & 0o077:
            raise ApplyError("Journal directory must be private (0700).")
        self.lock = open(directory / "lock", "a")
        os.chmod(directory / "lock", 0o600)
        try:
            fcntl.flock(self.lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            self.lock.close()
            raise ApplyError("Another apply process holds this journal.") from None
        self.path = directory / "journal.json"
        self.data = json.loads(self.path.read_text()) if self.path.exists() else {
            "version": 2, "binding": binding, "done": {}, "pending": None}
        if self.data.get("version") != 2 or self.data["binding"] != binding:
            self.close()
            raise ApplyError("Progress belongs to another archive, destination, or importer version.")
        save(self.path, self.data)

    def close(self):
        self.lock.close()

    def persist(self):
        """Durably save the current recovery state."""
        save(self.path, self.data)

    def reconcile(self, resolution):
        """Record an operator-verified result; never retry as part of recovery."""
        pending = self.data["pending"]
        if not pending or resolution.get("pending_sha256") != fingerprint(pending):
            raise ApplyError("Resolution does not identify the pending request.")
        if resolution.get("confirmed_not_written") is True and "response" not in resolution:
            pass
        elif isinstance(resolution.get("response"), dict) and "confirmed_not_written" not in resolution:
            response = resolution["response"]
            fields = {"CreateUser": ("member", "user", "id"), "CreateRoom": ("room", "id"),
                      "ImportMessage": ("messageId",)}.get(pending["method"], ())
            value = response
            for field in fields:
                value = value.get(field) if isinstance(value, dict) else None
            if fields and (not isinstance(value, str) or not value.strip()):
                raise ApplyError("Recovered response lacks the destination ID.")
            self.data["done"][pending["key"]] = response
        else:
            raise ApplyError("Resolution requires a recovered response or confirmed_not_written.")
        self.data.setdefault("reconciliations", []).append({"pending": pending, "resolution": resolution})
        self.data["pending"] = None
        self.persist()

    def write(self, key, service, method, request, api):
        if self.data["pending"]:
            raise ApplyError("Uncertain write: inspect journal pending; reconcile before resuming.")
        if key in self.data["done"]:
            return self.data["done"][key]
        self.data["pending"] = {"key": key, "service": service, "method": method, "request": request}
        if method == "ImportMessage":
            request = dict(request, idempotencyKey=str(uuid.uuid4()))
            self.data["pending"]["request"] = request
            self.data["pending"]["first_attempt_at"] = time.time()
        self.persist()
        response = api.call(service, method, request)
        self.data["done"][key] = response
        self.data["pending"] = None
        self.persist()
        return response

    def recover(self, users, rooms, api):
        """Resolve uncertain unique-name creates; retry idempotent operations.

        Only a persisted pending create permits name-based recovery. An
        arbitrary existing account or channel is never adopted by name.
        """
        pending = self.data["pending"]
        if not pending:
            return
        method, request = pending["method"], pending["request"]
        if method == "ImportMessage":
            first_attempt = pending.get("first_attempt_at")
            if (not request.get("idempotencyKey") or not isinstance(first_attempt, (int, float))
                    or not 0 <= time.time() - first_attempt < MESSAGE_RETRY_SECONDS):
                raise ApplyError("Uncertain message write outside the safe retry window: reconcile before resuming.")
            # Replay the exact persisted request and key. Never extend its window
            # or regenerate the body from current destination profile state.
            response = api.call("Message", "ImportMessage", request)
        elif method == "CreateUser":
            matches = [u for u in users.values() if u["login"].lower() == request["login"].lower()]
            response = {"member": {"user": matches[0]}} if matches else None
        elif method == "CreateRoom":
            matches = [r for r in rooms.values() if r["name"] == request["name"]]
            response = {"room": matches[0]} if matches else None
        elif method in ("AddMember", "ImportReaction", "AddVerifiedEmail"):
            response = None
        else:
            raise ApplyError("Unknown pending operation.")
        if response is not None:
            self.data["done"][pending["key"]] = response
        self.data["pending"] = None
        self.persist()


class SQLiteJournal(Journal):
    """Incremental durable journal; legacy JSON state is migrated on first use.

    Each transaction records the pending request or its result. FULL synchronous
    mode retains crash durability without serializing all earlier results.
    """

    def __init__(self, directory, binding):
        super().__init__(directory, binding)
        self.path = directory / "journal.sqlite"
        if self.path.is_symlink():
            self.close()
            raise ApplyError("Journal database must not be a symbolic link.")
        descriptor = os.open(self.path, os.O_CREAT | os.O_RDWR, 0o600)
        os.close(descriptor)
        os.chmod(self.path, 0o600)
        self.db = sqlite3.connect(self.path)
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.execute("PRAGMA synchronous=FULL")
        self.db.execute("CREATE TABLE IF NOT EXISTS state (key TEXT PRIMARY KEY, value TEXT NOT NULL)")
        self.db.execute("CREATE TABLE IF NOT EXISTS done (key TEXT PRIMARY KEY, value TEXT NOT NULL)")
        row = self.db.execute("SELECT value FROM state WHERE key='metadata'").fetchone()
        if row:
            self.data = json.loads(row[0])
            self.data["done"] = dict((key, json.loads(value)) for key, value in self.db.execute("SELECT key,value FROM done ORDER BY rowid"))
            if self.data["binding"] != binding:
                self.close()
                raise ApplyError("Progress belongs to another archive, destination, or importer version.")
        else:
            with self.db:
                self.db.executemany("INSERT INTO done VALUES (?,?)", ((key, json.dumps(value)) for key, value in self.data["done"].items()))
                metadata = {key: value for key, value in self.data.items() if key != "done"}
                self.db.execute("INSERT INTO state VALUES ('metadata',?)", (json.dumps(metadata),))
        self.saved_count = len(self.data["done"])
        self.persist()

    def persist(self):
        with self.db:
            if len(self.data["done"]) > self.saved_count:
                if len(self.data["done"]) != self.saved_count + 1:
                    raise ApplyError("Unexpected journal operation count.")
                key = next(reversed(self.data["done"]))
                self.db.execute("INSERT INTO done VALUES (?,?)", (key, json.dumps(self.data["done"][key])))
            metadata = {key: value for key, value in self.data.items() if key != "done"}
            self.db.execute("INSERT OR REPLACE INTO state VALUES ('metadata',?)", (json.dumps(metadata),))
        self.saved_count = len(self.data["done"])

    def close(self):
        if hasattr(self, "db"):
            self.db.close()
        super().close()


def escape(text):
    """Escape literal text for Chatto, whose Markdown disables backslash escapes.

    Entities decode after Markdown structure is parsed, preserving literal
    punctuation without adding visible backslashes or activating markup.
    """
    return re.sub(r"[&\\`*_{}\[\]<>~#!|()+.\-]", lambda match: "&#" + str(ord(match[0])) + ";", text)


def code_span(text):
    """Choose a delimiter absent from source; keep literal edge spaces/backticks."""
    if not isinstance(text, str) or "\n" in text or "\r" in text or not text:
        raise ApplyError("Unsupported multiline or empty inline code.")
    fence = "`" * max([1, *(len(run) + 1 for run in re.findall(r"`+", text))])
    pad = text.startswith("`") or text.endswith("`") or (text.startswith(" ") and text.endswith(" ") and text.strip())
    return fence + (" " if pad else "") + text + (" " if pad else "") + fence


def rich_link(url, label=None):
    """Preserve labels and URLs; unsupported protocols remain non-clickable code."""
    if not isinstance(url, str) or not url or any(ord(c) < 32 or ord(c) == 127 for c in url):
        raise ApplyError("Invalid rich-text link URL.")
    try:
        parsed = urlsplit(url)
    except ValueError:
        raise ApplyError("Invalid rich-text link URL.") from None
    if parsed.scheme.lower() not in {"http", "https", "mailto"}:
        return (escape(label) + " (" if label and label != url else "") + code_span(url) + (")" if label and label != url else "")
    if parsed.scheme.lower() in {"http", "https"} and not parsed.netloc:
        raise ApplyError("Invalid rich-text link URL.")
    # Angle destinations handle parentheses. Encode delimiters and whitespace;
    # preserve existing percent escapes and URL separators.
    destination = quote(url, safe="/:?#[]@!$&'()*+,;=%").replace("&", "&amp;")
    return "[" + escape(label or url) + "](<" + destination + ">)"


CHANNEL_NOTICES = {"channel_join", "channel_leave", "channel_name", "channel_topic",
                   "channel_purpose", "channel_archive", "channel_unarchive"}


def literal_slack_text(value):
    """Preserve exported text literally, without activating Slack or HTML markup."""
    if not isinstance(value, str):
        raise ApplyError("Invalid attachment or notice text.")
    return escape(value.replace("&lt;", "<").replace("&gt;", ">").replace("&amp;", "&"))


def attachment_summary(attachment, allow_missing_files):
    """Keep exported text and source URLs; never render HTML or embed media.

    This is an explicitly accepted summary, not a faithful Block Kit renderer.
    Interactive attachments and attachments without text remain unsupported.
    """
    if not isinstance(attachment, dict) or attachment.get("actions"):
        raise ApplyError("Interactive or invalid Slack attachment.")
    if attachment.get("is_msg_unfurl"):
        # Slack's fallback repeats the quoted message with notification metadata.
        # It is an alternative representation, not another paragraph to append.
        text = attachment.get("text") or attachment.get("fallback")
        if not isinstance(text, str) or not text.strip():
            raise ApplyError("Shared Slack message has no text summary.")
        parts = []
        author = attachment.get("author_name")
        if author:
            parts.append("**" + literal_slack_text(author) + "**")
        parts.append("\n".join("> " + line for line in literal_slack_text(text).split("\n")))
        url = attachment.get("original_url") or attachment.get("from_url") or attachment.get("title_link")
        if url:
            if not isinstance(url, str) or not re.fullmatch(r"https?://[^\s<>]+", url):
                raise ApplyError("Unsupported attachment source URL.")
            parts.append(rich_link(url, "Original Slack message"))
        files = attachment.get("files", [])
        if files and not allow_missing_files:
            raise ApplyError("Files require --allow-missing-files.")
        parts.extend("[Attachment unavailable in this import]" for _ in files)
        return "\n\n".join(parts)
    parts = []
    seen = set()
    primary = any(attachment.get(key) for key in ("pretext", "title", "text", "fields"))
    keys = ("pretext", "author_name", "author_subname", "title", "text", "footer")
    if not primary:
        keys += ("fallback",)
    for key in keys:
        value = attachment.get(key)
        if value:
            text = literal_slack_text(value)
            if text not in seen:
                parts.append(text)
                seen.add(text)
    for field in attachment.get("fields", []):
        if not isinstance(field, dict):
            raise ApplyError("Invalid Slack attachment field.")
        parts.append(" — ".join(literal_slack_text(field.get(key, "")) for key in ("title", "value")))
    if not any(part.strip() for part in parts):
        raise ApplyError("Slack attachment has no text summary.")
    for key in ("title_link", "original_url", "from_url", "author_link"):
        value = attachment.get(key)
        if value:
            if not isinstance(value, str) or not re.fullmatch(r"https?://[^\s<>]+", value):
                raise ApplyError("Unsupported attachment source URL.")
            text = rich_link(value, "Source")
            if text not in seen:
                parts.append(text)
                seen.add(text)
    files = attachment.get("files", [])
    if files and not allow_missing_files:
        raise ApplyError("Files require --allow-missing-files.")
    parts.extend("[Attachment unavailable in this import]" for _ in files)
    return "\n\n".join(parts)


def convert(message, logins, allow_missing_files, allow_attachment_summaries=False, channels=None, has_attachments=False):
    """Convert the supported rich-text subset; reject ambiguous input early.

    Rich text is authoritative. Legacy mrkdwn is deliberately rejected when
    formatting is present, until a complete parser can preserve its meaning.
    """
    def element(node):
        kind = node.get("type")
        if kind == "user":
            if node.get("user_id") not in logins:
                raise ApplyError("Unmapped mention.")
            return element({"type": "text", "text": "@" + logins[node["user_id"]], "style": node.get("style", {})})
        if kind == "text":
            text = escape(node.get("text", ""))
            style = node.get("style", {})
            if set(style) - {"bold", "italic", "strike", "code", "unlink", "underline"}:
                raise ApplyError("Unsupported rich-text style.")
            if style.get("code") or style.get("unlink"):
                raw = node.get("text", "")
                text = code_span(raw)
                for name, marker in (("bold", "**"), ("italic", "_"), ("strike", "~~"), ("underline", "__")):
                    if style.get(name):
                        text = marker + text + marker
                return text
            leading = text[:len(text) - len(text.lstrip())]
            trailing = text[len(text.rstrip()):]
            text = text.strip()
            if not text:
                return leading
            for name, marker in (("bold", "**"), ("italic", "_"), ("strike", "~~"), ("underline", "__")):
                if style.get(name):
                    text = marker + text + marker
            return leading + text + trailing
        if kind == "link":
            style = node.get("style", {})
            if set(style) - {"unlink", "bold", "italic", "strike", "code", "underline"}:
                raise ApplyError("Unsupported rich-text style.")
            if style.get("unlink") or style.get("code"):
                return element({"type": "text", "text": node.get("text") or node.get("url", ""),
                                "style": style})
            text = rich_link(node.get("url", ""), node.get("text"))
            for name, marker in (("bold", "**"), ("italic", "_"), ("strike", "~~"), ("underline", "__")):
                if style.get(name):
                    text = marker + text + marker
            return text
        if kind == "channel":
            channel = (channels or {}).get(node.get("channel_id"))
            if channel is None:
                raise ApplyError("Unmapped channel reference.")
            return escape("#" + channel)
        if kind == "message_mention":
            # Preserve the exported Slack permalink, including references to
            # messages outside this export. Do not invent a Chatto destination.
            return rich_link(node.get("url", ""), node.get("text") or "Slack message")
        if kind == "broadcast":
            if node.get("range") not in {"here", "channel", "everyone"}:
                raise ApplyError("Unsupported broadcast reference.")
            # Historical text, never a new all-members notification.
            return code_span("@" + node["range"])
        if kind == "color":
            value = node.get("value", "")
            if not re.fullmatch(r"#[0-9a-fA-F]{6}", value):
                raise ApplyError("Unsupported color value.")
            return code_span(value)
        if kind == "emoji":
            value = node.get("unicode", "")
            if value and re.fullmatch(r"[0-9a-fA-F]{2,6}(?:-[0-9a-fA-F]{2,6})*", value):
                points = [int(part, 16) for part in value.split("-")]
                if all(32 <= point <= 0x10ffff and not 0xd800 <= point <= 0xdfff and point != 127 for point in points):
                    return escape("".join(chr(point) for point in points))
            if re.fullmatch(r"[a-z0-9_+-]+", node.get("name", "")):
                return ":" + {"hugging_face": "hugs"}.get(node["name"], node["name"]) + ":"
        raise ApplyError("Unsupported rich-text element.")

    def section_text(section):
        if section.get("type") != "rich_text_section":
            raise ApplyError("Unsupported list item.")
        return "".join(element(node) for node in section.get("elements", []))

    def rich_sections(sections):
        """Render Slack's sibling list fragments with their explicit indentation."""
        output = []
        # Each level remembers its parent item's content indentation. This
        # handles ordered markers wider than three columns without flattening.
        indents = {0: 0}
        previous_list = False
        for section in sections:
            kind = section.get("type")
            if kind == "rich_text_list":
                depth, offset = section.get("indent", 0), section.get("offset", 0)
                if (type(depth) is not int or not 0 <= depth <= 20 or type(offset) is not int
                        or not 0 <= offset <= 999999998 or section.get("style") not in {"bullet", "ordered"}):
                    raise ApplyError("Unsupported list layout.")
                if not previous_list:
                    indents = {0: 0}
                if depth not in indents:
                    raise ApplyError("List indentation has no parent.")
                lines = []
                for i, item in enumerate(section.get("elements", [])):
                    marker = "- " if section["style"] == "bullet" else str(offset + i + 1) + ". "
                    prefix = " " * indents[depth]
                    content = section_text(item).split("\n")
                    lines.append(prefix + marker + content[0])
                    lines.extend(prefix + " " * len(marker) + line for line in content[1:])
                    indents[depth + 1] = indents[depth] + len(marker)
                    for deeper in list(indents):
                        if deeper > depth + 1:
                            del indents[deeper]
                rendered = "\n".join(lines)
                if previous_list:
                    output[-1] += "\n" + rendered
                else:
                    output.append(rendered)
                previous_list = True
                continue
            previous_list = False
            if kind == "rich_text_section":
                output.append(section_text(section))
            elif kind == "rich_text_quote":
                text = "".join(element(node) for node in section.get("elements", []))
                output.append("\n".join("> " + line for line in text.split("\n")))
            elif kind == "rich_text_preformatted":
                nodes = section.get("elements", [])
                if any(node.get("type") not in {"text", "link"} for node in nodes):
                    raise ApplyError("Unsupported code block element.")
                text = "".join(node.get("text") or (node.get("url", "") if node.get("type") == "link" else "") for node in nodes)
                fence = "`" * max([3, *(len(run) + 1 for run in re.findall(r"`+", text))])
                output.append(fence + "\n" + text + ("" if text.endswith("\n") else "\n") + fence)
            else:
                raise ApplyError("Unsupported rich-text section.")
        return "\n\n".join(output)

    subtype = message.get("subtype")
    if subtype == "tombstone":
        if any(message.get(key) for key in ("files", "attachments", "blocks")):
            raise ApplyError("Tombstone contains unsupported content.")
        # Slack's text is a deletion notice, not surviving authored content.
        # The deleted import writes no body at all.
        return ""
    if subtype not in {None, "file_share", "thread_broadcast", "me_message", "bot_message", "slackbot_response"} | CHANNEL_NOTICES:
        raise ApplyError("Unsupported message subtype.")
    if subtype == "thread_broadcast" and (not message.get("thread_ts") or message["thread_ts"] == message.get("ts")):
        raise ApplyError("Thread broadcast requires a thread root.")
    if message.get("attachments") and not allow_attachment_summaries:
        raise ApplyError("Slack attachments require --allow-attachment-summaries.")
    blocks = message.get("blocks")
    if subtype in CHANNEL_NOTICES:
        if blocks:
            raise ApplyError("Unsupported channel notice blocks.")
        text = message.get("text", "")
        if not text.strip():
            raise ApplyError("Channel notice has no text.")
        body = "[Slack channel notice: " + subtype + "]\n\n" + literal_slack_text(text)
    elif blocks:
        sections = []
        for block in blocks:
            if block.get("type") != "rich_text":
                raise ApplyError("Unsupported Slack block.")
            sections.append(rich_sections(block.get("elements", [])))
        body = "\n\n".join(sections)
    else:
        body = message.get("text", "")
        if re.search(r"[<>&*~`_]", body):
            raise ApplyError("Legacy mrkdwn requires conversion review.")
        body = escape(body)
    if subtype == "me_message":
        body = "[Slack action]\n\n" + body
    if message.get("attachments"):
        summaries = [attachment_summary(value, allow_missing_files) for value in message["attachments"]]
        body = "\n\n".join(part for part in [body, *summaries] if part.strip())
    if message.get("files"):
        if not allow_missing_files:
            raise ApplyError("Files require --allow-missing-files.")
        body += "\n\n[Attachment unavailable in this import]" * len(message["files"])
    if not body.strip() and not has_attachments:
        raise ApplyError("Converted message is empty.")
    if len(body.encode()) > 10000:
        raise OversizedBodyError("Converted message exceeds the body limit.")
    return body


def list_all(api, service, method, field):
    result = []
    while True:
        response = api.call(service, method, {"page": {"limit": 100, "offset": len(result)}})
        page = response.get(field, [])
        result.extend(page)
        if not response.get("page", {}).get("hasMore"):
            return result
        if not page:
            raise ApplyError("Invalid pagination response.")


def apply(plan, api, journal, allow_missing_files=False, skip_canvases=False, allow_attachment_summaries=False, file_importer=None, channel_prefix=""):
    """Import into a blank instance, with internal IDs and resumable creation."""
    if any(m["source"].get("subtype") == "tombstone" for m in plan["messages"]):
        # Require the new validation before any writes, including journal replay.
        # Older replicas may ignore deleted; the fake IDs cannot create a post.
        try:
            api.call("Message", "ImportMessage", {"roomId": "slack-import-probe", "authorId": "slack-import-probe",
                     "createdAt": "2025-01-01T00:00:00Z", "body": "probe", "deleted": True})
        except APIError as error:
            if error.code != "invalid_argument" or not error.invalid_deleted_content:
                raise ApplyError("Server must support deleted historical imports before importing or resuming.") from None
        else:
            raise ApplyError("Unexpected deleted-import capability response.")
    # Old servers can ignore unknown protobuf fields. Require the specific UUID
    # validation response before any write or uncertain-message replay.
    try:
        api.call("Message", "ImportMessage", {"roomId": "slack-import-probe", "authorId": "slack-import-probe",
                 "createdAt": "2025-01-01T00:00:00Z", "body": "probe", "idempotencyKey": "invalid"})
    except APIError as error:
        if error.code != "invalid_argument" or not error.invalid_idempotency_key:
            raise ApplyError("Server must support idempotent historical imports before importing or resuming.") from None
    else:
        raise ApplyError("Unexpected idempotency capability response.")
    allowed = {"deactivated_user_policy", "bot_identity_policy", "operator_avatar_import_missing",
               "reaction_import_required", "thread_import_required", "content_conversion_required",
               "uncertain_write_recovery_required", "file_hidden_by_limit",
               "file_download_or_local_mapping_required", "canvas_content_not_imported", "message_subtype_review",
               "slack_attachment_conversion_required"}
    if set(plan["summary"]["issues"]) - allowed:
        raise ApplyError("Export contains unsupported or incomplete source records.")
    if plan["canvases"] and not skip_canvases:
        raise ApplyError("Canvas metadata requires --skip-canvases.")
    users = {u["source_id"]: u for u in plan["users"]}
    needed = {u for room in plan["rooms"] for u in room["members"]}
    for message in plan["messages"]:
        needed.add(message["source_author_id"])
        for reaction in message["source"].get("reactions", []):
            needed.update(reaction["users"])
        # All rich-text user nodes, including unsupported containers, are inspected.
        def mentions(value):
            if isinstance(value, dict):
                if value.get("type") == "user":
                    needed.add(value.get("user_id"))
                for child in value.values():
                    mentions(child)
            elif isinstance(value, list):
                for child in value:
                    mentions(child)
        mentions(message["source"].get("blocks", []))
    logins, targets, user_requests = {}, {}, {}
    existing_users = list_all(api, "User", "ListUsers", "users")
    by_id = {u["user"]["id"]: u["user"] for u in existing_users}
    # Never merge identities because an email matches an existing account.
    # Check collisions before recovery or any destination mutation.
    email_owners = {email.lower(): member["user"]["id"] for member in existing_users
                    for email in member.get("verifiedEmails", [])}
    for user in users.values():
        email = user.get("verified_email")
        if not email:
            continue
        saved = journal.data["done"].get("user:" + user["source_id"])
        target = saved["member"]["user"]["id"] if saved else None
        owner = email_owners.get(email)
        if owner is not None and owner != target:
            raise ApplyError("Imported email conflicts with another destination account.")
        if "email:" + user["source_id"] in journal.data["done"] and owner != target:
            raise ApplyError("Imported email was removed from its destination account.")
    rooms = {r["id"]: r for r in list_all(api, "Room", "ListRooms", "rooms")}
    if "baseline" not in journal.data:
        if any((r["name"] not in {"general", "announcements"} and not r.get("universal"))
               or r.get("archived") for r in rooms.values()):
            raise ApplyError("Import requires a blank instance: channels other than the bootstrap defaults found.")
        # Bootstrap account provenance is not exposed by the operator API.
        # Treat the initial users and default channels as bootstrap state.
        journal.data["baseline"] = {"users": sorted(by_id), "rooms": sorted(rooms)}
        journal.persist()
    baseline = journal.data["baseline"]
    if not set(baseline["users"]) <= by_id.keys() or not set(baseline["rooms"]) <= rooms.keys():
        raise ApplyError("Destination bootstrap state changed; refusing to resume.")
    imported_users = {v["member"]["user"]["id"] for k, v in journal.data["done"].items() if k.startswith("user:")}
    imported_rooms = {v["room"]["id"] for k, v in journal.data["done"].items() if k.startswith("room:")}
    # Verify known destination identities before recovery can replay a write.
    if not imported_users <= by_id.keys() or not imported_rooms <= rooms.keys():
        raise ApplyError("Imported destination objects are missing; refusing to resume.")
    if journal.data["pending"] and journal.data["pending"]["method"] == "ImportMessage":
        if (set(by_id) != set(baseline["users"]) | imported_users
                or set(rooms) != set(baseline["rooms"]) | imported_rooms):
            raise ApplyError("Destination objects changed outside this import; refusing to retry.")
    journal.recover(by_id, rooms, api)
    imported_users = {v["member"]["user"]["id"] for k, v in journal.data["done"].items() if k.startswith("user:")}
    imported_rooms = {v["room"]["id"] for k, v in journal.data["done"].items() if k.startswith("room:")}
    if (set(by_id) != set(baseline["users"]) | imported_users
            or set(rooms) != set(baseline["rooms"]) | imported_rooms):
        raise ApplyError("Destination objects changed outside this import; refusing to resume.")
    occupied = {u["login"].lower() for u in by_id.values()}
    for source_id in sorted(needed):
        if source_id not in users:
            raise ApplyError("Unresolved source user.")
        user = users[source_id]
        saved = journal.data["done"].get("user:" + source_id)
        target = saved["member"]["user"]["id"] if saved else None
        if target:
            found = by_id.get(target)
            if not found or found.get("deleted"):
                raise ApplyError("Imported user is absent or deleted.")
            targets[source_id], logins[source_id] = target, found["login"]
        else:
            if user["deactivated"] or user["bot"]:
                raise ApplyError("Referenced bots and deactivated users are not supported yet.")
            login = user["proposed_login"]
            if not isinstance(login, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{1,31}", login) or login.endswith("."):
                raise ApplyError("Source username does not meet Chatto login limits; no automatic rename was made.")
            display = user["proposed_display_name"] or login
            if login.lower() in occupied or len(display) > 32:
                raise ApplyError("Source user conflicts with a bootstrap login or exceeds the display-name limit.")
            occupied.add(login.lower())
            logins[source_id] = login
            user_requests[source_id] = {"login": login, "displayName": display}
    channel_names = {room["source_id"]: channel_prefix + room["name"] for room in plan["rooms"]}
    bodies, message_files = {}, {}
    for message in plan["messages"]:
        source, files = (file_importer.prepare_message(message["source"], allow_missing_files)
                         if file_importer else (message["source"], []))
        key = message["source_key"]
        bodies[key] = convert(source, logins, allow_missing_files, allow_attachment_summaries, channel_names,
                              has_attachments=bool(files))
        message_files[key] = files
    seen = set()
    for message in plan["messages"]:
        root = message["thread_root_key"]
        if root and root != message["source_key"] and root not in seen:
            raise ApplyError("Thread root must precede its replies.")
        seen.add(message["source_key"])
    room_targets, room_requests = {}, {}
    occupied_rooms = {unicodedata.normalize("NFC", r["name"].strip()).casefold() for r in rooms.values()}
    for room in plan["rooms"]:
        key = room["source_id"]
        saved = journal.data["done"].get("room:" + key)
        target = saved["room"]["id"] if saved else None
        if room["archived"]:
            raise ApplyError("Archived source rooms are unsupported.")
        if target:
            if target not in rooms or rooms[target].get("archived") or rooms[target].get("universal"):
                raise ApplyError("Imported room must remain an active non-universal channel.")
            room_targets[key] = target
        else:
            name = unicodedata.normalize("NFC", (channel_prefix + room["name"]).strip())
            def text(value):
                return value.get("value", "") if isinstance(value, dict) else value or ""
            description = "\n\n".join(filter(None, [text(room["topic"]), text(room["purpose"])]))
            if (name.casefold() in occupied_rooms or not 1 <= len(name) <= 30 or len(description.encode()) > 500
                    or any(unicodedata.category(c).startswith("C") or c in "\u2028\u2029" for c in name)):
                raise ApplyError("Source room name conflicts or its metadata exceeds limits.")
            occupied_rooms.add(name.casefold())
            room_requests[key] = {"name": name, "description": description}
    # Invalid probe cannot write, even on servers that ignore the new fields.
    try:
        api.call("Message", "ImportMessage", {"roomId": "", "authorId": "", "createThread": True,
                                              "threadRootEventId": "probe"})
    except APIError as error:
        if error.code != "invalid_argument":
            raise ApplyError("Message import API is unavailable.") from None
    else:
        raise ApplyError("Unexpected capability probe response.")
    # Validate reaction names before creating any objects. The synthetic user
    # cannot exist, so valid emoji reach NOT_FOUND without recording a reaction.
    emojis = {r["name"] for m in plan["messages"] for r in m["source"].get("reactions", [])}
    for emoji in emojis:
        try:
            api.call("Message", "ImportReaction", {"roomId": "slack-import-probe",
                     "messageEventId": "slack-import-probe", "userId": "slack-import-probe",
                     "emoji": {"hugging_face": "hugs"}.get(emoji, emoji)})
        except APIError as error:
            if error.code != "not_found":
                raise ApplyError("Reaction emoji is unsupported by this server.") from None
        else:
            raise ApplyError("Unexpected reaction validation response.")
    try:
        api.call("Message", "ImportReaction", {})
    except APIError as error:
        if error.code != "invalid_argument":
            raise ApplyError("Reaction import API is unavailable.") from None
    else:
        raise ApplyError("Unexpected capability probe response.")
    if file_importer:
        try:
            api.call("Asset", "CreateUpload", {})
        except APIError as error:
            if error.code != "invalid_argument":
                raise ApplyError("Operator asset upload API is unavailable.") from None
        else:
            raise ApplyError("Unexpected asset upload capability response.")
    for key, request in user_requests.items():
        targets[key] = journal.write("user:" + key, "User", "CreateUser", request, api)["member"]["user"]["id"]
    # Email assignment is independently journaled: a lost CreateUser response
    # must not cause recovery to adopt a user whose email was never attached.
    for key, target in targets.items():
        email = users[key].get("verified_email")
        if email:
            journal.write("email:" + key, "User", "AddVerifiedEmail", {"userId": target, "email": email}, api)
    for key, request in room_requests.items():
        room_targets[key] = journal.write("room:" + key, "Room", "CreateRoom", request, api)["room"]["id"]
    for room in plan["rooms"]:
        for member in room["members"]:
            journal.write("member:" + room["source_id"] + ":" + member, "Room", "AddMember",
                          {"roomId": room_targets[room["source_id"]], "userId": targets[member]}, api)
    roots = {m["thread_root_key"] for m in plan["messages"] if m["thread_root_key"]}
    roots.update(m["source_key"] for m in plan["messages"] if "reply_count" in m["source"])
    message_ids = {}
    for message in plan["messages"]:
        key, root = message["source_key"], message["thread_root_key"]
        request = {"roomId": room_targets[message["source_channel_id"]],
                   "authorId": targets[message["source_author_id"]], "body": bodies[key],
                   "createdAt": message["created_at"]}
        if message["source"].get("subtype") == "tombstone":
            request["deleted"] = True
        elif message["edited_at"]:
            request["editedAt"] = message["edited_at"]
        if root and root != key:
            request["threadRootEventId"] = message_ids[root]
        elif key in roots:
            request["createThread"] = True
        # A recovered/completed message already owns its assets. Do not upload
        # replacements after pending message recovery or a successful earlier run.
        if message_files[key] and "message:" + key not in journal.data["done"]:
            request["attachmentAssetIds"] = [file_importer.upload(key + ":" + file_id, file_id,
                request["roomId"], request["authorId"], journal, api) for file_id in message_files[key]]
        message_ids[key] = journal.write("message:" + key, "Message", "ImportMessage", request, api)["messageId"]
        for reaction in message["source"].get("reactions", []):
            emoji = {"hugging_face": "hugs"}.get(reaction["name"], reaction["name"])
            for user in reaction["users"]:
                journal.write("reaction:" + key + ":" + emoji + ":" + user, "Message", "ImportReaction",
                              {"roomId": request["roomId"], "messageEventId": message_ids[key],
                               "userId": targets[user], "emoji": emoji}, api)
    result = {"users": len(targets), "rooms": len(room_targets), "messages": len(message_ids),
              "reactions": plan["summary"]["reactions"], "omitted_files": plan["summary"]["files"],
              "omitted_canvases": len(plan["canvases"]), "unused_users": len(users) - len(targets),
              "attachment_summaries": sum(len(m["source"].get("attachments", [])) for m in plan["messages"]),
              "tombstones": sum(m["source"].get("subtype") == "tombstone" for m in plan["messages"]),
              "channel_notices": sum(m["source"].get("subtype") in CHANNEL_NOTICES for m in plan["messages"]),
              "broadcasts_without_channel_echo": sum(m["source"].get("subtype") == "thread_broadcast" for m in plan["messages"]),
              "omitted_avatars": sum(u.get("has_avatar", False) for u in users.values())}
    journal.data["summary"] = result
    if "slack_workspace_ids" in plan:
        result["imported_emails"] = sum(bool(users[key].get("verified_email")) for key in targets)
    if file_importer:
        from slack_files import file_references
        result["attached_files"] = sum(len(files) for files in message_files.values())
        result["omitted_files"] = sum(f["id"] not in file_importer.ready for m in plan["messages"]
                                      for f in file_references(m["source"]))
        result["file_downloads"] = file_importer.report()
    journal.persist()
    return result
