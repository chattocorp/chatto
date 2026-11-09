"""Synthetic coverage for the pinned public Slack emoji lookup generator."""

import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("slack_emoji_generator", Path(__file__).with_name("gen-slack-emoji-map.py"))
generator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generator)


class SlackEmojiGeneratorTests(unittest.TestCase):
    def test_all_aliases_and_explicit_variations_are_generated(self):
        rows = [{"short_names": ["handshake", "shake"], "unified": "1F91D", "skin_variations": {
            "1F3FB-1F3FF": {"unified": "1FAF1-1F3FB-200D-1FAF2-1F3FF"},
            "1F3FD-1F3FD": {"unified": "1FAF1-1F3FD-200D-1FAF2-1F3FD"},
        }}, {"short_names": ["heart"], "unified": "2764-FE0F"}]
        result = generator.generate(rows)
        self.assertEqual(result, {
            "handshake": "🤝", "shake": "🤝", "heart": "❤️",
            "handshake::skin-tone-2-6": "🫱🏻‍🫲🏿", "shake::skin-tone-2-6": "🫱🏻‍🫲🏿",
            "handshake::skin-tone-4": "🫱🏽‍🫲🏽", "shake::skin-tone-4": "🫱🏽‍🫲🏽",
        })
        self.assertEqual(list(result), sorted(result))
        self.assertEqual(result, generator.generate(list(reversed(rows))))

    def test_conflicting_aliases_fail_instead_of_overwriting(self):
        with self.assertRaisesRegex(ValueError, "Conflicting"):
            generator.generate([{"short_names": ["same"], "unified": "1F91D"},
                                {"short_names": ["same"], "unified": "2764-FE0F"}])

    def test_invalid_modifier_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "skin tone"):
            generator.generate([{"short_names": ["hand"], "unified": "1F44D",
                                 "skin_variations": {"1F600": {"unified": "1F44D-1F600"}}}])

    def test_invalid_unicode_scalar_is_rejected(self):
        for value in ("D800", "110000", "not-hex"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                generator.unicode_sequence(value)
