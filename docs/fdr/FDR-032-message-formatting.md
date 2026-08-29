# FDR-032: Message Formatting

**Status:** Active
**Last reviewed:** 2026-08-29

## Overview

Message bodies are stored and exchanged as plain text while bundled clients render a deliberately limited Markdown subset. This gives people useful structure for chat, code, and compact tabular data without making message content depend on a client-specific rich-text document format.

## Behavior

- Messages support paragraphs, ATX headings, emphasis, links and autolinks, inline and fenced code, blockquotes, and ordered and unordered lists.
- Messages support GFM pipe tables with a header delimiter row, optional outer pipes, left/centre/right column alignment, inline formatting, and escaped pipes inside cells.
- Wide tables scroll horizontally inside the message instead of widening or clipping the conversation layout.
- Message source HTML, horizontal rules, reference-style links, and setext headings render as literal text rather than active formatting. Image syntax never loads or displays an image; its label and destination can fall back to an ordinary link.
- Backslashes normally remain literal so common chat text such as Windows paths and kaomoji is not unexpectedly changed. An escaped pipe inside a GFM table cell is still interpreted as cell content rather than a column boundary.
- Inline timestamp tokens render in the viewer's locale and timezone when supported by the client.
- Editing a message preserves the plain-text Markdown body contract; the bundled composer does not provide a spreadsheet-like table editor.
- The bundled client offers a syntax-highlighted Markdown source editor by default and an optional visual editor. Both edit the same Markdown body and provide the same formatting and composer features.
- Fenced code labelled with a supported language receives programming-language syntax highlighting while composing and after posting. Unlabelled and unsupported languages remain plain code.
- The app applies the editor choice to every registered Chatto server. It does not sync this App Preference to other browsers or devices.
- The app applies the sending keys to every registered Chatto server. Return sends by default. People can select the platform modifier plus Return instead.
- The key not assigned to sending performs the selected editor's normal Return action. In the visual editor that includes paragraph splitting, list continuation, leaving an empty list item, and new lines inside code blocks; Shift+Return remains a hard line break.
- Both editors provide toolbar actions to indent and outdent. In the visual editor they change list nesting. In the Markdown source editor they apply CodeMirror's normal line indentation to the current line or selection, as do Tab and Shift+Tab; autocomplete consumes Tab first when a suggestion is active.
- The composer shows message actions in its input row. A Formatting options control shows or hides the formatting toolbar above the input. The toolbar is hidden by default. The app stores the last selection in the browser and applies it to all composers.
- Touch-primary devices always use Return for editing and the visible Send button, even when Return-to-send is selected.

## Design Decisions

### 1. Plain-text Markdown is the interchange format

**Decision:** Message formatting is represented by Markdown in the existing plain-text body rather than by a client-specific rich-text document.
**Why:** Plain text remains portable across API clients, server versions, exports, and clients that implement only a subset of formatting. Unsupported syntax can still be displayed instead of making the message unreadable.
**Tradeoff:** Clients must implement compatible rendering themselves, and a rich composer cannot represent every valid source construct as a dedicated editing control.

### 2. The supported syntax is deliberately constrained

**Decision:** The bundled renderer enables common conversational structure and GFM tables while keeping source HTML, images, and several lower-value block constructs disabled.
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

## Related

- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-006 (@Mentions), FDR-030 (Inline Message Timestamps)
- **Guide:** [Format Messages](../../apps/docs-website/src/content/docs/getting-started/message-formatting.mdx)
