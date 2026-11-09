"""Synthetic avatar downloads: no source data or external requests."""
import io
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from slack_avatars import AvatarCache, AvatarDownloads, avatar_url, download_avatar
from slack_files import FileUnavailable


class AvatarTests(unittest.TestCase):
    def test_unavailable_original_uses_exported_thumbnail(self):
        with tempfile.TemporaryDirectory() as directory:
            cache=AvatarCache(Path(directory)/'cache')
            with patch.object(cache,'get',side_effect=[FileUnavailable('file_size_limit'),{'size':32}]) as get:
                self.assertEqual(download_avatar(cache,{'avatar_urls':['https://avatars.slack-edge.com/original','https://avatars.slack-edge.com/512']}),{'size':32})
                self.assertEqual(get.call_count,2)
                self.assertEqual(get.call_args.args[0]['url_private_download'],'https://avatars.slack-edge.com/512')

    def test_largest_image_and_host_restrictions(self):
        self.assertEqual(avatar_url({'image_72':'small','image_512':'large'}), 'large')
        self.assertEqual(avatar_url({'image_original':'original','image_512':'large'}), 'original')
        for url in ('http://avatars.slack-edge.com/a', 'https://avatars.slack-edge.com.evil.test/a',
                    'https://localhost/a', 'https://secure.gravatar.com:444/a', 'https://user@secure.gravatar.com/a'):
            with self.assertRaises(FileUnavailable): AvatarCache.validate_url(url)
        AvatarCache.validate_url('https://i0.wp.com/a.slack-edge.com/default.png')
        with self.assertRaises(FileUnavailable): AvatarCache.validate_url('https://i0.wp.com/untrusted.test/private')
        with self.assertRaises(FileUnavailable): AvatarCache.validate_url('https://i0.wp.com')

    def test_cache_and_changed_url(self):
        with tempfile.TemporaryDirectory() as directory:
            def opened(url):
                data = io.BytesIO(b'avatar image bytes')
                data.status, data.headers = 200, {}
                return data
            users = [{'source_id':'U1','avatar_url':'https://avatars.slack-edge.com/one'}]
            with patch.object(AvatarCache, '_open', side_effect=opened) as fetch:
                first = AvatarDownloads(users, Path(directory)/'cache')
                second = AvatarDownloads(users, Path(directory)/'cache')
                self.assertEqual(first.ready, second.ready)
                self.assertEqual(fetch.call_count, 1)
                users[0]['avatar_url'] = 'https://avatars.slack-edge.com/two'
                AvatarDownloads(users, Path(directory)/'cache')
                self.assertEqual(fetch.call_count, 2)
                self.assertNotIn('https://', ''.join(p.read_text() for p in Path(directory).rglob('*.json')))

    def test_failure_has_no_url(self):
        with tempfile.TemporaryDirectory() as directory:
            result = AvatarDownloads([{'source_id':'U1','avatar_url':'https://untrusted.test/private'}], Path(directory)/'cache')
            self.assertEqual(result.failures, {'U1':'untrusted_avatar_url'})

    def test_redirect_is_checked(self):
        import urllib.error
        from email.message import Message
        headers = Message()
        headers['Location'] = 'https://untrusted.test/private'
        with tempfile.TemporaryDirectory() as directory:
            cache = AvatarCache(Path(directory)/'cache')
            with patch.object(cache.opener, 'open', side_effect=urllib.error.HTTPError('url',302,'redirect',headers,None)) as opened:
                with self.assertRaisesRegex(FileUnavailable,'untrusted_avatar_url'):
                    cache._open('https://secure.gravatar.com/avatar/a')
                self.assertEqual(opened.call_count,1)
