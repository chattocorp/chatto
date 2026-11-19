"""Synthetic rich-text examples; no private export fixtures."""

import unittest

from slack_apply import ApplyError, code_span, convert, escape, rich_link


def text(value, **style):
    return {"type": "text", "text": value, "style": style}


def section(*elements):
    return {"type": "rich_text_section", "elements": list(elements)}


def render(*sections):
    return convert({"blocks": [{"type": "rich_text", "elements": list(sections)}]},
                   {"U1": "alice"}, False, channels={"C1": "slack-general"})


class RichTextTests(unittest.TestCase):
    def test_underline_preserves_other_styles_and_whitespace(self):
        self.assertEqual(render(section(text(" moo ", underline=True))), " __moo__ ")
        self.assertEqual(render(section(text("moo", underline=True, bold=True))), "__**moo**__")
        self.assertEqual(render(section(text("moo", underline=True, italic=True))), "___moo___")
        self.assertEqual(render(section(text("moo", underline=True, code=True))), "__`moo`__")
        self.assertEqual(render(section(text("before "), text(" moo ", underline=True), text(" after"))),
                         "before  __moo__  after")
        self.assertEqual(render(section({"type": "user", "user_id": "U1", "style": {"underline": True}})), "__@alice__")
        for style in ({"underline": True}, {"underline": True, "unlink": True}):
            expected = "__`moo`__" if style.get("unlink") else "__[moo](<https://example.org>)__"
            self.assertEqual(render(section({"type": "link", "url": "https://example.org", "text": "moo", "style": style})), expected)

    def test_nested_lists_number_offsets_and_continuation_lines(self):
        self.assertEqual(render(
            {"type": "rich_text_list", "style": "ordered", "offset": 99, "elements": [section(text("parent\ncontinued"))]},
            {"type": "rich_text_list", "style": "bullet", "indent": 1, "elements": [section(text("child"))]},
            {"type": "rich_text_list", "style": "ordered", "offset": 100, "elements": [section(text("next"))]},
        ), "100. parent\n     continued\n     - child\n101. next")

    def test_quote_lines_and_paragraph_boundaries(self):
        self.assertEqual(render(section(text("before")),
                                {"type": "rich_text_quote", "elements": [text("one\n\ntwo")]},
                                section(text("after"))), "before\n\n> one\n> \n> two\n\nafter")

    def test_code_delimiters_preserve_source(self):
        self.assertEqual(code_span("a`b"), "``a`b``")
        self.assertEqual(code_span("`edge`"), "`` `edge` ``")
        self.assertEqual(code_span(" both "), "`  both  `")
        self.assertEqual(render({"type": "rich_text_preformatted", "elements": [text("a\n```\nb &amp; <x>")]}),
                         "````\na\n```\nb &amp; <x>\n````")

    def test_links_preserve_parentheses_spaces_and_labels(self):
        self.assertEqual(rich_link("https://example.org/a(b)?q=x y", "[label]"),
                         "[&#91;label&#93;](<https://example.org/a(b)?q=x%20y>)")
        self.assertEqual(rich_link("mailto:user@example.org", "Email"), "[Email](<mailto:user@example.org>)")
        self.assertEqual(rich_link("tel:+123", "Call"), "Call (`tel:+123`)")
        self.assertEqual(rich_link("javascript:alert(1)", "Unsafe"), "Unsafe (`javascript:alert(1)`)")
        with self.assertRaises(ApplyError):
            rich_link("https://example.org/\nInjected")

    def test_references_and_unlinked_text(self):
        self.assertEqual(render(section({"type": "channel", "channel_id": "C1"})), "&#35;slack&#45;general")
        self.assertEqual(render(section({"type": "broadcast", "range": "here"})), "`@here`")
        self.assertEqual(render(section({"type": "message_mention", "url": "https://example.org/message", "text": "Earlier"})),
                         "[Earlier](<https://example.org/message>)")
        self.assertEqual(render(section({"type": "link", "url": "https://example.org", "text": "literal", "style": {"unlink": True}})), "`literal`")
        with self.assertRaisesRegex(ApplyError, "Unmapped channel"):
            render(section({"type": "channel", "channel_id": "missing"}))

    def test_literal_punctuation_cannot_activate_markdown(self):
        self.assertEqual(escape("*x* &amp; <img>"), "&#42;x&#42; &#38;amp; &#60;img&#62;")
        self.assertNotIn("\\", escape("[x](y)"))

    def test_unicode_emoji_preserves_skin_tones_and_code_links_stay_literal(self):
        self.assertEqual(render(section({"type": "emoji", "name": "wave::skin-tone-3", "unicode": "1f44b-1f3fc"})), "👋🏼")
        self.assertEqual(render({"type": "rich_text_preformatted", "elements": [{"type": "link", "url": "https://example.org"}]}),
                         "```\nhttps://example.org\n```")

    def test_unknown_shapes_remain_errors(self):
        for value in ({"type": "rich_text_list", "style": "bullet", "indent": 3, "elements": [section(text("orphan"))]},
                      {"type": "rich_text_list", "style": "other", "elements": []},
                      {"type": "rich_text_preformatted", "elements": [{"type": "user", "user_id": "U1"}]},
                      section(text("unknown", unknown=True)), section({"type": "unknown"})):
            with self.assertRaises(ApplyError):
                render(value)


if __name__ == "__main__":
    unittest.main()
