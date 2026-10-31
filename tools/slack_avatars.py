"""Explicit, credential-free avatar acquisition from Slack and Gravatar.

Only exact HTTPS hosts are allowed, including after redirects. Cache keys bind
the URL without storing it. FileCache enforces byte budgets and private files.
"""

import hashlib
import urllib.parse

from slack_files import FileCache, FileUnavailable


def avatar_urls(profile):
    """Return the original image and thumbnails, largest first, without repeats."""
    keys = [k for k in profile if k.startswith("image_") and k[6:].isdigit()]
    keys.sort(key=lambda k: int(k[6:]), reverse=True)
    return list(dict.fromkeys(profile[k] for k in ["image_original", *keys]
                             if isinstance(profile.get(k), str) and profile[k]))


def avatar_url(profile):
    """Return the preferred exported avatar URL, if present."""
    return next(iter(avatar_urls(profile)), None)


def download_avatar(cache, user):
    """Fall back to smaller exported variants if the preferred image fails."""
    candidates = user.get('avatar_urls') or [user.get('avatar_url')]
    failure = FileUnavailable('no_avatar_url')
    for url in candidates:
        if not url:
            continue
        try:
            key = hashlib.sha256(url.encode()).hexdigest()
            return cache.get({'id': key, 'name': 'avatar', 'mimetype': 'application/octet-stream',
                              'url_private_download': url})
        except FileUnavailable as error:
            failure = error
    raise failure


class AvatarCache(FileCache):
    @staticmethod
    def validate_url(url):
        try:
            value = urllib.parse.urlsplit(url)
            hosts = {"avatars.slack-edge.com", "a.slack-edge.com", "secure.gravatar.com"}
            proxy = value.hostname in {"i0.wp.com", "i1.wp.com", "i2.wp.com"}
            proxy_origin = value.path.split('/')[1] if value.path.startswith('/') else ''
            allowed = value.hostname in hosts or (proxy and proxy_origin in hosts)
            if (value.scheme != "https" or not allowed
                    or value.port not in (None, 443) or value.username or value.password or value.fragment
                    or any(ord(c) < 33 or ord(c) == 127 for c in url)):
                raise ValueError()
        except (TypeError, ValueError):
            raise FileUnavailable("untrusted_avatar_url") from None


class AvatarDownloads:
    def __init__(self, users, directory):
        self.ready, self.failures = {}, {}
        cache = AvatarCache(directory, 5 * 1024**2, 1024**3)
        for user in users:
            url = user.get("avatar_url")
            if not url:
                continue
            try:
                self.ready[user["source_id"]] = download_avatar(cache, user)
            except FileUnavailable as error:
                self.failures[user["source_id"]] = str(error)
