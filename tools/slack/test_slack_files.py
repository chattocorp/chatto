"""Synthetic file download and upload recovery tests. No private export fixtures."""

import base64
import hashlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch
import urllib.error

from slack_apply import APIError, ApplyError, Journal, apply, convert
from slack_files import FileCache, FileImports, FileUnavailable, file_inventory
import test_slack_apply as apply_tests


def file_record(key="F1", data=b"image bytes"):
    return {"id": key, "name": "image.png", "mimetype": "image/png", "size": len(data),
            "url_private_download": "https://files.slack.com/files-pri/example?token=private-token"}


def response(data=b"image bytes", content_type="image/png", length=True):
    result = io.BytesIO(data)
    result.status = 200
    result.headers = {"Content-Type": content_type}
    if length:
        result.headers["Content-Length"] = str(len(data))
    return result


class DownloadTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.cache = FileCache(Path(self.temp.name) / "progress", 1000, 2000)
        self.cache.opener = Mock()

    def test_download_cache_and_corrupt_cache_recovery(self):
        self.cache.opener.open.return_value = response()
        item = self.cache.get(file_record())
        self.assertEqual(Path(item["path"]).read_bytes(), b"image bytes")
        self.assertEqual(Path(item["path"]).stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.cache.get(file_record()), item)
        self.assertEqual(self.cache.opener.open.call_count, 1)
        metadata = next(self.cache.directory.glob("*.json")).read_text()
        self.assertNotIn("private-token", metadata)
        Path(item["path"]).write_bytes(b"corrupt")
        self.cache.opener.open.return_value = response()
        self.cache.get(file_record())
        self.assertEqual(self.cache.opener.open.call_count, 2)

    def test_signed_url_download_has_no_bearer_or_cookie(self):
        self.cache.opener.open.return_value = response()
        self.cache.get(file_record())
        request = self.cache.opener.open.call_args.args[0]
        self.assertIsNone(request.get_header("Authorization"))
        self.assertIsNone(request.get_header("Cookie"))

    def test_external_urls_and_redirects_never_receive_requests(self):
        for url in ("http://files.slack.com/a", "https://files.slack.com.example.org/a", "https://user@files.slack.com/a", "https://127.0.0.1/a"):
            with self.assertRaisesRegex(FileUnavailable, "untrusted"):
                self.cache.get(dict(file_record(), url_private_download=url))
        self.cache.opener.open.assert_not_called()
        self.cache.opener.open.side_effect = urllib.error.HTTPError("private-url", 302, "private", {"Location": "https://other.example/a?token=private"}, None)
        with self.assertRaisesRegex(FileUnavailable, "untrusted"):
            self.cache.get(file_record())
        self.assertEqual(self.cache.opener.open.call_count, 1)

    def test_allowed_redirect_and_rate_limit_retry(self):
        self.cache.opener.open.side_effect = [
            urllib.error.HTTPError("private", 302, "private", {"Location": "https://files-origin.slack.com/a?token=private"}, None),
            urllib.error.HTTPError("private", 429, "private", {"Retry-After": "2"}, None), response()]
        with patch("slack_files.time.sleep") as sleep:
            self.cache.get(file_record())
        sleep.assert_called_once_with(2)

    def test_html_size_and_budget_failures_remove_partial_files(self):
        cases = [(b"<html>login</html>", "text/html", 1000, 2000, "html_instead"),
                 (b"x" * 20, "image/png", 10, 2000, "file_size_limit"),
                 (b"x" * 20, "image/png", 1000, 10, "budget")]
        for data, mime, limit, budget, error in cases:
            with self.subTest(error=error):
                self.cache.max_file_bytes, self.cache.max_download_bytes = limit, budget
                self.cache.downloaded_bytes = 0
                self.cache.opener.open.return_value = response(data, mime, length=False)
                item = file_record()
                del item["size"]
                with self.assertRaisesRegex(FileUnavailable, error):
                    self.cache.get(item)
                self.assertEqual(list(self.cache.directory.glob(".partial-*")), [])
                self.assertEqual(list(self.cache.directory.glob("*.data")), [])

    def test_truncated_file_and_access_errors_are_safe(self):
        self.cache.opener.open.return_value = response(b"short", length=False)
        with self.assertRaisesRegex(FileUnavailable, "size_mismatch"):
            self.cache.get(file_record())
        self.cache.opener.open.side_effect = urllib.error.HTTPError("private-token", 403, "private-secret", {}, None)
        with self.assertRaises(FileUnavailable) as caught:
            self.cache.get(file_record())
        self.assertEqual(str(caught.exception), "download_access_denied")

    def test_stub_deduplication_and_nested_file_messages(self):
        full = file_record()
        plan = {"messages": [{"source": {"files": [{"id": "F1", "mode": "hidden_by_limit"}]}},
                             {"source": {"attachments": [{"files": [full]}]}}]}
        self.assertEqual(file_inventory(plan), {"F1": full})
        self.cache.opener.open.return_value = response()
        files = FileImports(plan, self.cache)
        self.assertEqual(self.cache.opener.open.call_count, 1)
        message, ids = files.prepare_message(plan["messages"][1]["source"], False)
        self.assertEqual(ids, ["F1"])
        self.assertEqual(convert(message, {}, False, has_attachments=True), "")
        self.assertNotIn("private-token", json.dumps(files.binding()))

    def test_sparse_file_references_do_not_hide_later_download_metadata(self):
        full = file_record()
        sparse = {"id": full["id"], "name": full["name"]}
        for references in ([sparse, full], [full, sparse]):
            plan = {"messages": [{"source": {"files": [item]}} for item in references]}
            self.assertEqual(file_inventory(plan), {full["id"]: full})
        self.assertEqual(sparse, {"id": full["id"], "name": full["name"]})

    def test_complementary_file_metadata_is_merged_but_conflicts_fail(self):
        full = file_record()
        first = {k:v for k,v in full.items() if k != "url_private_download"}
        later = {"id": full["id"], "url_private_download": full["url_private_download"]}
        plan = {"messages": [{"source": {"files": [first, later]}}]}
        self.assertEqual(file_inventory(plan), {full["id"]: full})
        later["size"] = full["size"] + 1
        with self.assertRaisesRegex(ApplyError, "metadata conflicts"):
            file_inventory(plan)

    def test_deleted_file_observations_keep_complete_metadata(self):
        full = dict(file_record(), mode="hosted")
        deleted = {"id": full["id"], "mode": "tombstone"}
        hidden = {"id": full["id"], "mode": "hidden_by_limit"}
        for references in ([deleted, full, hidden], [full, deleted], [hidden, deleted, full]):
            plan = {"messages": [{"source": {"files": references}}]}
            self.assertEqual(file_inventory(plan), {full["id"]: full})
        with self.assertRaisesRegex(FileUnavailable, "file_deleted"):
            self.cache.get(deleted)
        self.cache.opener.open.assert_not_called()

    def test_hidden_files_and_tombstones_stay_explicit(self):
        plan = {"messages": [{"source": {"files": [{"id": "F1", "mode": "hidden_by_limit"}]}}]}
        files = FileImports(plan, self.cache)
        self.assertEqual(files.report()["failures"], {"hidden_by_limit": 1})
        self.cache.opener.open.assert_not_called()
        with self.assertRaisesRegex(ApplyError, "unavailable"):
            files.prepare_message(plan["messages"][0]["source"], False)
        message, ids = files.prepare_message(plan["messages"][0]["source"], True)
        self.assertIn("unavailable", convert(message, {}, True))
        with self.assertRaisesRegex(ApplyError, "Tombstone"):
            files.prepare_message(dict(message, subtype="tombstone"), True)


class AssetOperator(apply_tests.FakeOperator):
    def __init__(self):
        super().__init__()
        self.sessions = {}
        self.fail_at = None
        self.created = 0
        self.completed = 0

    def call(self, service, method, request):
        if service != "Asset":
            return super().call(service, method, request)
        if method == "CreateUpload":
            if not request:
                raise APIError("invalid_argument")
            self.created += 1
            key = "upload-" + str(self.created)
            self.sessions[key] = {"request": request, "data": b"", "asset": None}
            result = {"upload": {"uploadId": key, "maxChunkSize": 4}}
        elif request["uploadId"] not in self.sessions:
            raise APIError("not_found")
        elif method == "UploadChunk":
            session = self.sessions[request["uploadId"]]
            assert request["offset"] == len(session["data"])
            chunk = base64.b64decode(request["content"])
            assert hashlib.sha256(chunk).hexdigest() == request["chunkSha256"]
            session["data"] += chunk
            result = {}
        elif method == "CompleteUpload":
            session = self.sessions[request["uploadId"]]
            assert len(session["data"]) == session["request"]["size"]
            assert hashlib.sha256(session["data"]).hexdigest() == session["request"]["sha256"]
            if not session["asset"]:
                self.completed += 1
                session["asset"] = "asset-" + str(self.completed)
            result = {"assetId": session["asset"]}
        elif method == "CancelUpload":
            del self.sessions[request["uploadId"]]
            result = {}
        if self.fail_at == method:
            self.fail_at = None
            raise OSError("simulated response loss")
        return result


class UploadTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name) / "progress"
        cache = FileCache(self.directory)
        cache.opener = Mock()
        cache.opener.open.return_value = response()
        self.files = FileImports({"messages": [{"source": {"files": [file_record()]}}]}, cache)
        self.api = AssetOperator()

    def journal(self):
        return Journal(self.directory, {"test": True})

    def test_lost_create_chunk_and_completion_responses(self):
        for method in ("CreateUpload", "UploadChunk", "CompleteUpload"):
            with self.subTest(method=method):
                self.api = AssetOperator()
                self.api.fail_at = method
                key = method
                journal = self.journal()
                with self.assertRaises(OSError):
                    self.files.upload(key, "F1", "room", "author", journal, self.api)
                journal.close()
                journal = self.journal()
                asset = self.files.upload(key, "F1", "room", "author", journal, self.api)
                self.assertEqual(asset, "asset-1")
                self.assertEqual(self.api.completed, 1)
                self.assertEqual(self.api.created, 1 if method == "CompleteUpload" else 2)
                self.assertEqual(self.files.upload(key, "F1", "room", "author", journal, self.api), asset)
                journal.close()

    def test_expired_completed_assets_get_replaced(self):
        journal = self.journal()
        self.addCleanup(journal.close)
        self.files.upload("key", "F1", "room", "author", journal, self.api)
        journal.data["file_uploads"]["key"]["completed_at"] -= 24 * 3600
        self.assertEqual(self.files.upload("key", "F1", "room", "author", journal, self.api), "asset-2")

    def test_file_only_import_and_pending_message_recovery_reuse_asset(self):
        fixture = apply_tests.ApplyTests()
        fixture.setUp()
        self.addCleanup(fixture.doCleanups)
        plan = fixture.plan
        plan["messages"][0]["source"] = {"files": [file_record()]}
        plan["summary"]["files"] = 1
        journal = self.journal()
        self.api.lose_message_response = True
        with self.assertRaises(OSError):
            apply(plan, self.api, journal, file_importer=self.files)
        self.assertEqual(self.api.messages[0]["body"], "")
        self.assertEqual(self.api.messages[0]["attachmentAssetIds"], ["asset-1"])
        journal.close()
        journal = self.journal()
        self.addCleanup(journal.close)
        self.api.lose_message_response = False
        result = apply(plan, self.api, journal, file_importer=self.files)
        self.assertEqual(result["attached_files"], 1)
        self.assertEqual(result["omitted_files"], 0)
        self.assertEqual(self.api.completed, 1)
        self.assertEqual(len(self.api.messages), 2)
        apply(plan, self.api, journal, file_importer=self.files)
        self.assertEqual(self.api.completed, 1)


if __name__ == "__main__":
    unittest.main()
