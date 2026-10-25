"""Import Slack records through Chatto's local operator Connect API.

No Slack downloads occur. A durable pending record precedes every mutation;
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
import tempfile
import time
import unicodedata
import uuid

# Stop before the server's fixed 30-minute window, allowing transport margin.
MESSAGE_RETRY_SECONDS = 29 * 60
INVALID_IDEMPOTENCY_KEY = "idempotency_key must be a non-zero UUID in hyphenated form"


class ApplyError(ValueError):
    """A safe, content-free diagnostic for the operator."""


class APIError(ApplyError):
    def __init__(self, code, invalid_idempotency_key=False):
        super().__init__("Operator request failed (" + code + ").")
        self.code = code
        self.invalid_idempotency_key = invalid_idempotency_key


class Operator:
    """JSON Connect transport restricted to a local Unix socket."""

    def __init__(self, path):
        self.path = str(Path(path).resolve())

    def call(self, service, method, body):
        connection = http.client.HTTPConnection("localhost", timeout=30)
        connection.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        connection.sock.settimeout(30)
        try:
            connection.sock.connect(self.path)
            connection.request("POST", "/api/connect/chatto.operator.v1.Operator" + service + "Service/" + method,
                               json.dumps(body).encode(),
                               {"Content-Type": "application/json", "Connect-Protocol-Version": "1"})
            response = connection.getresponse()
            value = json.loads(response.read())
            if response.status != 200:
                code = value.get("code")
                raise APIError(code if code in {"invalid_argument", "not_found", "already_exists", "unimplemented"} else "request_error",
                               value.get("message") == INVALID_IDEMPOTENCY_KEY)
            return value
        finally:
            connection.close()


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
        save(self.path, self.data)

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
        save(self.path, self.data)
        response = api.call(service, method, request)
        self.data["done"][key] = response
        self.data["pending"] = None
        save(self.path, self.data)
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
        elif method in ("AddMember", "ImportReaction"):
            response = None
        else:
            raise ApplyError("Unknown pending operation.")
        if response is not None:
            self.data["done"][pending["key"]] = response
        self.data["pending"] = None
        save(self.path, self.data)


def escape(text):
    """Keep literal rich-text content from becoming Markdown instructions."""
    return re.sub(r"([\\`*_{}\[\]<>~#!|()+.\-])", r"\\\1", text)


def convert(message, logins, allow_missing_files):
    """Convert the supported rich-text subset; reject ambiguous input early.

    Rich text is authoritative. Legacy mrkdwn is deliberately rejected when
    formatting is present, until a complete parser can preserve its meaning.
    """
    def element(node):
        kind = node.get("type")
        if kind == "user":
            if node.get("user_id") not in logins:
                raise ApplyError("Unmapped mention.")
            return "@" + logins[node["user_id"]]
        if kind == "text":
            text = escape(node.get("text", ""))
            style = node.get("style", {})
            if set(style) - {"bold", "italic", "strike", "code"}:
                raise ApplyError("Unsupported rich-text style.")
            if style.get("code"):
                raw = node.get("text", "")
                if "`" in raw or "\n" in raw or any(style.get(k) for k in ("bold", "italic", "strike")):
                    raise ApplyError("Unsupported inline code delimiter.")
                return "`" + raw + "`"
            leading = text[:len(text) - len(text.lstrip())]
            trailing = text[len(text.rstrip()):]
            text = text.strip()
            if not text:
                return leading
            for name, marker in (("bold", "**"), ("italic", "_"), ("strike", "~~")):
                if style.get(name):
                    text = marker + text + marker
            return leading + text + trailing
        if kind == "link":
            url = node.get("url", "")
            if not re.fullmatch(r"https?://[^\s<>\\()]+", url):
                raise ApplyError("Unsupported link URL.")
            return "[" + escape(node.get("text") or url) + "](" + url + ")"
        if kind == "emoji" and re.fullmatch(r"[a-z0-9_+-]+", node.get("name", "")):
            return ":" + {"hugging_face": "hugs"}.get(node["name"], node["name"]) + ":"
        raise ApplyError("Unsupported rich-text element.")

    if message.get("subtype") not in (None, "file_share") or message.get("attachments"):
        raise ApplyError("Unsupported message subtype or Slack attachment.")
    blocks = message.get("blocks")
    if blocks:
        sections = []
        for block in blocks:
            if block.get("type") != "rich_text":
                raise ApplyError("Unsupported Slack block.")
            for section in block.get("elements", []):
                if section.get("type") != "rich_text_section":
                    raise ApplyError("Unsupported rich-text section.")
                sections.append("".join(element(node) for node in section.get("elements", [])))
        body = "\n".join(sections)
    else:
        body = message.get("text", "")
        if re.search(r"[<>&*~`_]", body):
            raise ApplyError("Legacy mrkdwn requires conversion review.")
        body = escape(body)
    if message.get("files"):
        if not allow_missing_files:
            raise ApplyError("Files require --allow-missing-files.")
        body += "\n\n[Attachment unavailable in this import]" * len(message["files"])
    if not body.strip() or len(body.encode()) > 10000:
        raise ApplyError("Converted message is empty or exceeds the body limit.")
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


def apply(plan, api, journal, allow_missing_files=False, skip_canvases=False):
    """Import into a blank instance, with internal IDs and resumable creation."""
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
               "file_download_or_local_mapping_required", "canvas_content_not_imported", "message_subtype_review"}
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
    rooms = {r["id"]: r for r in list_all(api, "Room", "ListRooms", "rooms")}
    if "baseline" not in journal.data:
        if any(r["name"] not in {"general", "announcements"} or r.get("archived") for r in rooms.values()):
            raise ApplyError("Import requires a blank instance: channels other than the bootstrap defaults found.")
        # Bootstrap account provenance is not exposed by the operator API.
        # Treat the initial users and default channels as bootstrap state.
        journal.data["baseline"] = {"users": sorted(by_id), "rooms": sorted(rooms)}
        save(journal.path, journal.data)
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
            suffix = hashlib.sha256(source_id.encode()).hexdigest()[:8]
            stem = re.sub(r"[^a-z0-9_-]", "-", (user["proposed_login"] or "user").lower()).strip("-_")[:20] or "user"
            login = stem + "-" + suffix
            display = user["proposed_display_name"] or login
            if login in occupied or len(display) > 32:
                raise ApplyError("Source user conflicts with a bootstrap login or exceeds the display-name limit.")
            occupied.add(login)
            logins[source_id] = login
            user_requests[source_id] = {"login": login, "displayName": display}
    bodies = {m["source_key"]: convert(m["source"], logins, allow_missing_files) for m in plan["messages"]}
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
            name = unicodedata.normalize("NFC", ("slack-" + room["name"]).strip())
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
    for key, request in user_requests.items():
        targets[key] = journal.write("user:" + key, "User", "CreateUser", request, api)["member"]["user"]["id"]
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
        if message["edited_at"]:
            request["editedAt"] = message["edited_at"]
        if root and root != key:
            request["threadRootEventId"] = message_ids[root]
        elif key in roots:
            request["createThread"] = True
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
              "omitted_avatars": sum(u.get("has_avatar", False) for u in users.values())}
    journal.data["summary"] = result
    save(journal.path, journal.data)
    return result
