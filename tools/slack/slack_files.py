"""Download exported Slack file URLs and upload cached bytes through the operator.

No API bearer token is used. Signed URLs stay in memory. Cache files and upload
checkpoints live in the source progress directory, never the system temp folder.
"""

import base64
import copy
import hashlib
import http.client
import json
import os
from pathlib import Path
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

from slack_apply import APIError, ApplyError, fingerprint, save
from slack_api import NoRedirect

FILE_HOSTS = {"files.slack.com", "files-origin.slack.com"}


class FileUnavailable(ApplyError):
    """A content-free reason that can be counted in an omission report."""


def file_references(message):
    """Include files nested in legacy Slack attachment containers."""
    return message.get("files", []) + [f for a in message.get("attachments", []) for f in a.get("files", [])]


def file_inventory(plan):
    """Combine compatible references by ID without changing source records.

    A message can contain only an ID while another reference supplies download
    metadata. Hidden stubs never override an available file description.
    """
    result = {}
    for message in plan["messages"]:
        for item in file_references(message["source"]):
            key = item.get("id") if isinstance(item, dict) else None
            if not isinstance(key, str) or not key:
                raise ApplyError("Slack file reference has no ID.")
            previous = result.get(key)
            # A deleted/hidden observation can coexist with a complete file
            # reference. Keep the complete metadata so cached bytes survive.
            partial_modes = {"hidden_by_limit", "tombstone"}
            if previous and previous.get("mode") not in partial_modes and item.get("mode") not in partial_modes:
                for field in ("size", "name", "mimetype", "mode", "is_external", "filetype"):
                    if previous.get(field) not in (None, "") and item.get(field) not in (None, "") and previous[field] != item[field]:
                        raise ApplyError("Slack file metadata conflicts across references.")
                for field, value in item.items():
                    if previous.get(field) in (None, ""):
                        previous[field] = value
            if (previous is None or previous.get("mode") == "hidden_by_limit"
                    or previous.get("mode") == "tombstone" and item.get("mode") not in partial_modes):
                result[key] = dict(item)
    return result


def validate_url(url):
    """Signed credentials may only travel over HTTPS to exact Slack file hosts."""
    try:
        value = urllib.parse.urlsplit(url)
        if (value.scheme != "https" or value.hostname not in FILE_HOSTS or value.port not in (None, 443)
                or value.username or value.password or value.fragment
                or any(ord(c) < 33 or ord(c) == 127 for c in url)):
            raise ValueError()
    except (TypeError, ValueError):
        raise FileUnavailable("untrusted_download_url") from None


class FileCache:
    """Bounded streaming downloads with atomic completion and hash verification."""

    def __init__(self, directory, max_file_bytes=25 * 1024**2, max_download_bytes=1024**3):
        self.directory = Path(directory) / "files"
        self.directory.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.directory.mkdir(mode=0o700, exist_ok=True)
        for path in (self.directory.parent, self.directory):
            if path.is_symlink() or path.stat().st_mode & 0o077:
                raise ApplyError("File cache directories must be private (0700), without symlinks.")
        self.max_file_bytes = max_file_bytes
        self.max_download_bytes = max_download_bytes
        self.downloaded_bytes = 0
        self.opener = urllib.request.build_opener(NoRedirect)

    def _open(self, url):
        for _ in range(6):
            self.validate_url(url)
            try:
                return self.opener.open(urllib.request.Request(url, headers={"Accept-Encoding": "identity"}), timeout=30)
            except urllib.error.HTTPError as error:
                if error.code not in {301, 302, 303, 307, 308}:
                    raise
                target = error.headers.get("Location")
                error.close()
                if not target:
                    raise FileUnavailable("invalid_download_redirect") from None
                url = urllib.parse.urljoin(url, target)
        raise FileUnavailable("too_many_download_redirects")

    validate_url = staticmethod(validate_url)

    def get(self, item):
        if item.get("mode") == "tombstone":
            raise FileUnavailable("file_deleted")
        if item.get("mode") == "hidden_by_limit":
            raise FileUnavailable("hidden_by_limit")
        if item.get("is_external") or item.get("mode") == "external" or item.get("filetype") == "remote":
            raise FileUnavailable("external_file")
        expected_size = item.get("size")
        if expected_size is not None and (type(expected_size) is not int or expected_size < 0):
            raise FileUnavailable("invalid_file_size")
        if expected_size is not None and expected_size > self.max_file_bytes:
            raise FileUnavailable("file_size_limit")
        key = hashlib.sha256(item["id"].encode()).hexdigest()
        path, metadata = self.directory / (key + ".data"), self.directory / (key + ".json")
        for entry in (path, metadata):
            if entry.is_symlink():
                raise ApplyError("File cache contains a symbolic link.")
        source = fingerprint({k: item.get(k) for k in ("id", "size", "name", "mimetype")})
        if path.exists() and metadata.exists():
            try:
                saved = json.loads(metadata.read_text())
                with path.open("rb") as data:
                    digest = hashlib.file_digest(data, "sha256").hexdigest()
                if (saved["source"] == source and saved["sha256"] == digest
                        and saved["size"] == path.stat().st_size <= self.max_file_bytes):
                    return dict(saved, path=str(path))
            except (OSError, ValueError, KeyError, TypeError):
                pass  # A corrupt/incomplete cache entry is downloaded again.
        url = item.get("url_private_download") or item.get("url_private")
        if not isinstance(url, str) or not url:
            raise FileUnavailable("no_download_url")
        self.validate_url(url)
        if self.downloaded_bytes >= self.max_download_bytes:
            raise FileUnavailable("download_budget_exceeded")
        filename = (item.get("name") or "attachment").replace("\\", "/").rsplit("/", 1)[-1]
        filename = "".join(c for c in filename if ord(c) >= 32 and ord(c) != 127)[:255] or "attachment"
        content_type = item.get("mimetype") or "application/octet-stream"
        for attempt in range(3):
            temporary = None
            try:
                with self._open(url) as response:
                    if response.status != 200:
                        raise FileUnavailable("unexpected_download_status")
                    declared = response.headers.get("Content-Length")
                    if declared and (not declared.isdigit() or int(declared) > self.max_file_bytes):
                        raise FileUnavailable("file_size_limit")
                    if declared and expected_size is not None and int(declared) != expected_size:
                        raise FileUnavailable("file_size_mismatch")
                    descriptor, temporary = tempfile.mkstemp(dir=self.directory, prefix=".partial-")
                    size, digest, first = 0, hashlib.sha256(), True
                    with os.fdopen(descriptor, "wb") as output:
                        while chunk := response.read(64 * 1024):
                            size += len(chunk)
                            self.downloaded_bytes += len(chunk)
                            if size > self.max_file_bytes:
                                raise FileUnavailable("file_size_limit")
                            if self.downloaded_bytes > self.max_download_bytes:
                                raise FileUnavailable("download_budget_exceeded")
                            if first:
                                head = chunk[:4096].lstrip().lower()
                                html = "text/html" in response.headers.get("Content-Type", "").lower() or head.startswith((b"<!doctype html", b"<html"))
                                if html and content_type not in {"text/html", "application/xhtml+xml"}:
                                    raise FileUnavailable("html_instead_of_file")
                                if html and (b"slack.com/signin" in head or b"<title>slack" in head):
                                    raise FileUnavailable("html_instead_of_file")
                                first = False
                            digest.update(chunk)
                            output.write(chunk)
                        output.flush()
                        os.fsync(output.fileno())
                    if (expected_size is not None and size != expected_size) or (declared and size != int(declared)):
                        raise FileUnavailable("file_size_mismatch")
                    saved = {"source": source, "filename": filename, "content_type": content_type,
                             "size": size, "sha256": digest.hexdigest()}
                    os.replace(temporary, path)
                    save(metadata, saved)
                    return dict(saved, path=str(path))
            except urllib.error.HTTPError as error:
                code, retry_after = error.code, error.headers.get("Retry-After", "")
                error.close()
                if attempt < 2 and (code == 429 or 500 <= code <= 599):
                    delay = int(retry_after) if retry_after.isdigit() else 2**attempt
                    if delay <= 60:
                        time.sleep(max(1, delay))
                        continue
                reason = {401: "download_access_denied", 403: "download_access_denied", 404: "file_not_found", 410: "file_deleted", 429: "download_rate_limited"}.get(code, "download_http_error")
                raise FileUnavailable(reason) from None
            except (OSError, http.client.HTTPException):
                if attempt < 2:
                    time.sleep(2**attempt)
                    continue
                raise FileUnavailable("download_transport_error") from None
            finally:
                if temporary and os.path.exists(temporary):
                    os.unlink(temporary)


class FileImports:
    """Prepare downloads once and attach room/author-specific assets per message."""

    def __init__(self, plan, cache):
        self.ready, self.failures = {}, {}
        for key, item in file_inventory(plan).items():
            try:
                self.ready[key] = cache.get(item)
            except FileUnavailable as error:
                self.failures[key] = str(error)

    def report(self):
        from collections import Counter
        return {"unique_files": len(self.ready) + len(self.failures), "downloaded_or_cached_files": len(self.ready),
                "available_bytes": sum(f["size"] for f in self.ready.values()),
                "failures": dict(sorted(Counter(self.failures.values()).items()))}

    def binding(self):
        """Freeze content and omissions across resume, without storing signed URLs."""
        return {"ready": {key: {k: value[k] for k in ("sha256", "size", "filename", "content_type")}
                          for key, value in self.ready.items()}, "missing": sorted(self.failures)}

    def prepare_message(self, message, allow_missing):
        """Remove only downloadable references; retain unavailable placeholders."""
        references = file_references(message)
        if message.get("subtype") == "tombstone" and references:
            raise ApplyError("Tombstone contains unsupported content.")
        ids = list(dict.fromkeys(f["id"] for f in references if f["id"] in self.ready))
        if any(f["id"] not in self.ready for f in references) and not allow_missing:
            raise ApplyError("Files are unavailable; inspect --download-files --preflight or allow missing files.")
        if len(ids) > 10:
            raise ApplyError("Message exceeds Chatto's limit of 10 file attachments.")
        result = copy.deepcopy(message)
        result["files"] = [f for f in result.get("files", []) if f["id"] not in self.ready]
        attachments = []
        for attachment in result.get("attachments", []):
            had_files = bool(attachment.get("files"))
            attachment["files"] = [f for f in attachment.get("files", []) if f["id"] not in self.ready]
            # Pure file containers need no empty textual attachment summary.
            if had_files and set(attachment) <= {"files", "id"}:
                result["files"].extend(attachment["files"])
                continue
            attachments.append(attachment)
        if "attachments" in result:
            result["attachments"] = attachments
        return result, ids

    def upload(self, key, file_id, room, author, journal, api):
        """Restart uncertain chunks; replay completion; never change pending posts.

        A lost create response can leave an unknown session for server expiry.
        Open known sessions are cancelled on resume because chunks cannot be
        replayed. Completed assets are reused for at most 23 hours, below the
        server's 24-hour unattached-asset TTL. Message recovery runs first.
        """
        item = self.ready[file_id]
        binding = {"room": room, "author": author, "sha256": item["sha256"], "size": item["size"],
                   "filename": item["filename"], "content_type": item["content_type"]}
        uploads = journal.data.setdefault("file_uploads", {})
        state = uploads.get(key)
        if journal.data.get("pending"):
            raise ApplyError("Resolve the pending message before uploading attachments.")
        if state and state["binding"] != binding:
            raise ApplyError("Cached attachment changed after its upload began.")
        if state and state.get("asset_id") and 0 <= time.time() - state["completed_at"] < 23 * 3600:
            return state["asset_id"]
        if state and state.get("phase") == "completing":
            try:
                response = api.call("Asset", "CompleteUpload", {"uploadId": state["upload_id"]})
                state.update(phase="complete", asset_id=response["assetId"], completed_at=state["completion_started_at"])
                journal.persist()
                if 0 <= time.time() - state["completed_at"] < 23 * 3600:
                    return state["asset_id"]
            except APIError as error:
                if error.code != "not_found":
                    raise
        if state and state.get("phase") == "open":
            try:
                api.call("Asset", "CancelUpload", {"uploadId": state["upload_id"]})
            except APIError as error:
                if error.code != "not_found":
                    raise
        state = {"binding": binding, "phase": "creating"}
        uploads[key] = state
        journal.persist()
        session = api.call("Asset", "CreateUpload", {"roomId": room, "authorId": author, "filename": item["filename"],
                           "contentType": item["content_type"], "size": item["size"], "sha256": item["sha256"]})["upload"]
        state.update(phase="open", upload_id=session["uploadId"])
        journal.persist()
        chunk_size = min(int(session["maxChunkSize"]), 1024 * 1024)
        if chunk_size <= 0:
            raise ApplyError("Invalid operator upload chunk limit.")
        digest, offset = hashlib.sha256(), 0
        with open(item["path"], "rb") as data:
            while chunk := data.read(chunk_size):
                digest.update(chunk)
                api.call("Asset", "UploadChunk", {"uploadId": session["uploadId"], "offset": offset,
                         "content": base64.b64encode(chunk).decode(), "chunkSha256": hashlib.sha256(chunk).hexdigest()})
                offset += len(chunk)
        if offset != item["size"] or digest.hexdigest() != item["sha256"]:
            raise ApplyError("Cached file changed during upload.")
        state.update(phase="completing", completion_started_at=time.time())
        journal.persist()
        response = api.call("Asset", "CompleteUpload", {"uploadId": session["uploadId"]})
        state.update(phase="complete", asset_id=response["assetId"], completed_at=state["completion_started_at"])
        journal.persist()
        return state["asset_id"]
