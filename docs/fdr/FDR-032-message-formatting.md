# FDR-032: Message Formatting

**Status:** Active
**Last reviewed:** 2026-10-29

## Overview

Message bodies are stored and exchanged as plain text while bundled clients render a deliberately limited Markdown subset. This gives people useful structure for chat, code, and compact tabular data without making message content depend on a client-specific rich-text document format.

## Behavior

- Automatic links require `www.` or an allowed protocol: `https://`, `http://`, or `mailto:`. Bare domains, filenames, email addresses without `mailto:`, and protocol-relative URLs stay plain text. Explicit Markdown links also allow root-relative paths that start with one `/`, such as `[room](/chat/example.com/R8rLgzhPkYja7Gs)`. These paths do not produce link previews, and the visual editor preserves them. Other protocols stay unlinked. This rule prevents filenames such as `agents.md` and `foo.py` from becoming links. The source editor uses the same prefix rule for URL highlighting. Selecting a `mailto:` link opens the configured mail application.
- Messages support paragraphs, ATX headings, emphasis, links and autolinks, inline code, fenced and indented code blocks, blockquotes, and ordered and unordered lists.
- Built-in emoji shortcodes, such as `:wave:`, `:heart:`, and `:+1:`, render as Unicode emoji. Names are case-sensitive and use the gemoji catalogue, including its aliases, plus the entries in `tools/emoji-additions.json`. The additions supply newer emoji missing from the released dependency; `mise codegen-emoji` includes them in the server, client, and converter maps. The picker and shortcode resolver use the same base catalogue. Display depends on the device's emoji font. Adjacent tags work without spaces. Unknown names stay literal. Code, automatic URL labels, link destinations, and link titles stay unchanged. Explicit link labels, quotes, tables, footnotes, and inline Markdown previews support shortcodes.
- Typed shortcodes remain in the stored message and in both editors. Editing, reopening, and copying the full message source retain them. Existing Unicode emoji and the picker's Unicode insertion still work. Other clients can show shortcode text without a server upgrade.
- The smiley button beside the attachment and timestamp controls opens the same emoji picker used for reactions. Selecting an emoji inserts Unicode at the cursor or replaces selected text, then returns focus to the editor. Both editors support this in new messages, thread replies, and message edits. Search, skin-tone choices, and the per-server Recently Used list are shared with the reaction picker. Touch-primary devices show the picker in a bottom sheet. The button is disabled when the message input is disabled.
- Shortcodes directly beside Unicode letters, numbers, combining marks, or literal underscores stay literal. For example, `omg:poop:foo` stays unchanged. Whitespace and punctuation permit conversion, so `(:wave:)!` and `:wave::heart:` work. Markdown formatting still applies around tags, such as `_:wave:_` for an italic emoji. Autocomplete uses the same opening boundary and shortcode name characters. Suggestions appear after two name characters, including signed aliases such as `:+1`.
- Double underscores (`__text__`) mean underline, including in existing messages. Double asterisks (`**text**`) mean bold. Underline uses the normal emphasis boundaries and does not interpret underscores inside inline code, code blocks, or link destinations. Both editors provide an Underline toolbar button and Ctrl+U (⌘U on macOS). The visual editor retains underline when messages are saved, reopened, or switched to Markdown.
- Messages support GFM pipe tables with a header delimiter row, optional outer pipes, left/centre/right column alignment, inline formatting, and escaped pipes inside cells.
- Messages, server welcome messages, and Markdown attachment previews support named footnotes. Notes are numbered in reference order and appear after the text. Each reference links to its note, and each note has a return link for every reference. These links stay inside the current message or preview.
- Both composer editors can insert a footnote from the formatting toolbar. The visual editor shows numbered markers and editable notes, supports repeated references, and retains the named syntax when switching modes or editing a message. Typing `[^label]` in the visual editor creates a note or refers to its existing definition.
- Wide tables scroll horizontally inside the message instead of widening or clipping the conversation layout.
- Message source HTML, horizontal rules, reference-style links, and setext headings render as literal text rather than active formatting. Image syntax never loads or displays an image; its label and destination can fall back to an ordinary link.
- Backslashes normally remain literal so common chat text such as Windows paths and kaomoji is not unexpectedly changed. An escaped pipe inside a GFM table cell is still interpreted as cell content rather than a column boundary.
- Inline timestamp tokens render in the viewer's locale and timezone when supported by the client.
- Editing a message preserves the plain-text Markdown body contract; the bundled composer does not provide a spreadsheet-like table editor.
- The bundled client offers a syntax-highlighted Markdown source editor by default and an optional visual editor. Both edit the same Markdown body and provide the same formatting and composer features.
- The Markdown source editor enables spellcheck, autocorrection, sentence capitalization, and writing suggestions for chat text. Availability depends on browser support and keyboard settings.
- Fenced code labelled with a supported language receives programming-language syntax highlighting while composing and after posting. In the visual editor, pasting into an unlabelled code block or leaving a changed block starts language detection. A successful guess becomes the block's saved language and appears in the language selector. Users can override the guess, including with **TEXT** for plain text. Detection uses a fixed set of preferred languages and does not change blocks merely opened for editing. Unlabelled blocks without a match and unsupported languages remain plain code. Detection does not delay sending.
- Rendered code blocks use the visual editor's spacing without line numbers. Each block has a button to copy its code without the Markdown fence or display-only tab spacing.
- Code-copy buttons and shared copy controls briefly show a checkmark after the
  clipboard write succeeds. Failed copies keep the copy icon and show an error.
- Fenced blocks labelled `ansi` show static ANSI art after posting. They support terminal colors, Unicode text, cursor movement, and erase commands. Each block uses an 80-column, 100-row screen with no scrollback. Unused bottom rows are hidden. Wide or tall art scrolls inside the block. The first four ANSI blocks in a message render as art; additional blocks show their source. Copy retains the escape codes. Artwork uses a fixed black background in both themes. It cannot blink, open terminal links, access the clipboard, or make external connections.
- The message menu offers **Copy message text** to copy the full Markdown source. When text is selected wholly within that message body, **Copy selected text** appears immediately above it and copies only the selected text. A selection outside the message or across messages does not show this action. The menu keeps the selected text when it opens, even if focus changes.
- Plain URLs to known users, rooms, and servers appear as inline pills with the current name and an avatar or icon. This includes angle-bracket URLs and links to other registered servers. Explicit Markdown links retain their labels and normal link display. Selecting a message pill opens the entity. Right-clicking or long-pressing it opens entity actions.
- The visual composer previews automatic entity links as pills immediately after paste, without a trailing space. Selecting a preview or moving the caret into its text reveals the URL for editing. Sending, copying, or switching editor modes retains the URL, not the displayed entity name.
- Entity pills use data already available to the client. Unknown or unavailable entities keep the original link display. A later name change updates the pill; loss of the entity restores the original link. Rendering does not discover unknown servers or fetch missing profiles or rooms. Images come from the known entity's avatar or server icon, as on other identity surfaces.
- Only plain entity destinations become pills. Message and thread permalinks, management routes, and URLs with a query or fragment remain ordinary links. Links in code remain literal text. Entity links do not send mention notifications.
- Right-clicking an ordinary link in a message body opens the message menu. **Copy Link** copies that link's URL; **Copy Message Link** copies the message permalink.
- The app applies the editor choice to every registered Chatto server. It does not sync this App Preference to other browsers or devices.
- The app applies the sending keys to every registered Chatto server. Return sends by default. People can select the platform modifier plus Return instead.
- The key not assigned to sending performs the selected editor's normal Return action. In the visual editor that includes paragraph splitting, list continuation, leaving an empty list item, and new lines inside code blocks; Shift+Return remains a hard line break.
- Both editors provide toolbar actions to indent and outdent. In the visual editor they change list nesting. In the Markdown source editor they change the indentation of the current line or selected lines.
- In the Markdown source editor, Tab and Shift+Tab move keyboard focus without changing the text. Autocomplete still handles these keys first: Tab selects a suggestion or completes a mention, and repeated Tab presses cycle through matching mentions. An Escape prefix is not required for normal focus navigation.
- The composer shows message actions below the text in narrow panes. In wider panes, actions share the input row until the draft grows beyond one line. They then stay below the text until the draft is cleared or sent, or its destination or edit mode changes. This prevents the layout from switching repeatedly when the extra width removes a line wrap.
- A Formatting options control shows or hides the formatting toolbar above the input. The toolbar is hidden by default. The app stores the last selection in the browser and applies it to all composers. Both toolbars use larger buttons and icons when touch is the primary input, at every viewport width. Mouse-primary hybrid devices use compact controls. Spacing changes automatically with the primary input. The formatting toolbar scrolls horizontally when its controls do not fit.
- Touch-primary devices always use Return for editing and the visible Send button, even when Return-to-send is selected.

## Design Decisions

### 1. Plain-text Markdown is the interchange format

**Decision:** Message formatting is represented by Markdown in the existing plain-text body rather than by a client-specific rich-text document.
**Why:** Plain text remains portable across API clients, server versions, exports, and clients that implement only a subset of formatting. Unsupported syntax can still be displayed instead of making the message unreadable.
**Tradeoff:** Clients must implement compatible rendering themselves, and a rich composer cannot represent every valid source construct as a dedicated editing control.

### 2. The supported syntax is deliberately constrained

**Decision:** The bundled renderer enables common conversational structure, GFM tables, and named footnotes while keeping source HTML, images, and several lower-value block constructs disabled.
**Why:** A small reviewed output surface is easier to keep predictable and safe in user-authored messages. File attachments already provide the supported path for images.
**Tradeoff:** Markdown copied from other applications may contain valid constructs that Chatto intentionally shows as literal text.

### 3. Tables favour readable data over layout control

**Decision:** Tables use semantic rows, headers, cells, and GFM column alignment, with native horizontal scrolling when their content is wider than the message.
**Why:** Tables are useful for compact comparisons and status data, but message authors should not be able to use them to force the conversation column wider or create arbitrary page layouts.
**Tradeoff:** Large tables require horizontal scrolling on narrow screens and are less convenient to author in the bundled visual editor than ordinary prose.

### 4. Composer presentation is an App Preference

**Decision:** The bundled client supplies visual and Markdown source editors for the same message body. App Preferences stores the editor and send-key choices on the Composer page. It also stores the formatting-toolbar state when a person changes it in the composer. Markdown, Return-to-send, and a hidden formatting toolbar are the defaults when a preference is absent or invalid. The key that does not send keeps the editor's normal Return behavior. Each editor uses its own indent model. The visual editor changes list structure. The Markdown editor changes source-line indentation.
**Why:** People can choose direct source control or a syntax-free editing experience without changing the server contract, message history, or what other clients receive. They can also preserve their preferred chat shortcut and familiar keyboard editing behavior in paragraphs, lists, or code blocks.
**Tradeoff:** App Preferences do not follow a person to another browser or device. Each editor must have the same composer integrations and formatting controls. A person must open the toolbar before they can select a formatting control. The indent controls operate differently in each editor. The alternate Return shortcut changes meaning with the selected send mode.

### 5. ANSI blocks show a static terminal screen

**Decision:** Interpret terminal drawing commands only in `ansi` fences and show the final screen as selectable text. Use fixed screen dimensions and a limit on the number of rendered blocks.
**Why:** People and bots can share colored art and cursor-based drawings through the existing message body. Fixed limits keep each message's display work bounded. Static output needs no active terminal session.
**Tradeoff:** Terminal animations and raw CP437 file imports are not supported. Send Unicode text with actual escape characters. The composer shows source. Other clients can show the source if they do not support ANSI art.

### 6. Emoji shortcodes are display syntax

**Decision:** Resolve built-in shortcodes at word boundaries when displaying Markdown. Keep their source text in the message body and leave unknown names visible. Apply the same opening boundary to autocomplete.
**Why:** People and bots can send readable emoji names through the existing message API. Boundaries prevent accidental conversion inside words or identifiers. Shared client rules keep name lookup consistent across hosts without changing stored messages or contacting an external service.
**Tradeoff:** Older clients can show the literal shortcode. The visual editor keeps typed shortcodes as text. Custom emoji assets and stable references are not implemented; their identity and rename rules need a separate design.

### 7. Normal links carry entity references

**Decision:** Keep URLs and Markdown links in the message body. Replace only automatic URL displays with known entity names. Explicit Markdown labels keep their normal link display.
**Why:** Existing links work across clients without a custom reference format. Current names identify plain URL destinations. Keeping explicit labels preserves the author's sentence.
**Tradeoff:** The pill can differ between readers because their clients know different entities. The URL remains in the message source and returns when the target is unavailable.

## Related

- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-005 (Reactions), FDR-006 (@Mentions), FDR-030 (Inline Message Timestamps)
- **Guide:** [Format Messages](../../apps/docs-website/src/content/docs/getting-started/message-formatting.mdx)
