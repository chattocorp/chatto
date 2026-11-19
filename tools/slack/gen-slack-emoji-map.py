#!/usr/bin/env python3
"""Generate the offline Slack alias/skin-tone lookup from pinned public data.

Only this maintenance command contacts GitHub. No Slack export is read or sent.
Update the revision and digest together after reviewing upstream data changes.
"""

import hashlib
import json
from pathlib import Path
import re
import urllib.request

REVISION = "13ee711e222ea17fe537bfea953c687866f16411"
SHA256 = "8736d721991e48c41c57acc46b65e5d32258a45e4bcdebcc6012f420f81e775d"
URL = f"https://raw.githubusercontent.com/iamcal/emoji-data/{REVISION}/emoji.json"


def unicode_sequence(value):
    """Decode the upstream complete sequence, without changing selectors or ZWJs."""
    if not re.fullmatch(r"[0-9A-F]{4,6}(?:-[0-9A-F]{4,6})*", value):
        raise ValueError("Invalid upstream Unicode sequence")
    points = [int(part, 16) for part in value.split("-")]
    if any(point > 0x10FFFF or 0xD800 <= point <= 0xDFFF for point in points):
        raise ValueError("Invalid upstream Unicode scalar")
    return "".join(chr(point) for point in points)


def generate(rows):
    """Map every upstream alias and explicit skin variation; reject collisions."""
    result = {}

    def add(name, value):
        sequence = unicode_sequence(value)
        if name in result and result[name] != sequence:
            raise ValueError("Conflicting upstream emoji aliases")
        result[name] = sequence

    for row in rows:
        for name in row["short_names"]:
            if not re.fullmatch(r"[A-Za-z0-9_+-]{1,128}", name):
                raise ValueError("Invalid upstream emoji name")
            add(name, row["unified"])
            for modifiers, variation in row.get("skin_variations", {}).items():
                points = [int(part, 16) for part in modifiers.split("-")]
                if not points or any(point < 0x1F3FB or point > 0x1F3FF for point in points):
                    raise ValueError("Invalid upstream skin tone")
                tones = [str(point - 0x1F3FB + 2) for point in points]
                # Match Slack/Chatto's single suffix for equal tones, and the
                # ordered suffix for different tones on a multi-person emoji.
                suffix = tones[:1] if len(set(tones)) == 1 else tones
                add(name + "::skin-tone-" + "-".join(suffix), variation["unified"])
    return dict(sorted(result.items()))


def main():
    with urllib.request.urlopen(URL, timeout=30) as response:
        raw = response.read()
    if hashlib.sha256(raw).hexdigest() != SHA256:
        raise ValueError("Upstream data digest does not match the pinned revision")
    result = {"source": URL, "sha256": SHA256, "emoji": generate(json.loads(raw))}
    Path(__file__).with_name("slack-emoji-map.json").write_text(
        json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"Generated {len(result['emoji'])} Slack emoji mappings")


if __name__ == "__main__":
    main()
