# FDR-032: Message Formatting

**Status:** Active
**Last reviewed:** 2026-07-19

## Overview

Message bodies are stored and exchanged as plain text while bundled clients render a deliberately limited Markdown subset. This gives people useful structure for chat, code, and compact tabular data without making message content depend on a client-specific rich-text document format.

## Behavior

- Messages support paragraphs, ATX headings, emphasis, links and autolinks, inline and fenced code, blockquotes, and ordered and unordered lists.
- Messages support GFM pipe tables with a header delimiter row, optional outer pipes, left/centre/right column alignment, inline formatting, and escaped pipes inside cells.
- Wide tables scroll horizontally inside the message instead of widening or clipping the conversation layout.
- Message source HTML, images, horizontal rules, reference-style links, and setext headings render as literal text rather than active formatting.
- Backslashes normally remain literal so common chat text such as Windows paths and kaomoji is not unexpectedly changed. An escaped pipe inside a GFM table cell is still interpreted as cell content rather than a column boundary.
- Inline timestamp tokens render in the viewer's locale and timezone when supported by the client.
- Editing a message preserves the plain-text Markdown body contract; the bundled composer does not provide a spreadsheet-like table editor.

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
**Tradeoff:** Large tables require horizontal scrolling on narrow screens and are less convenient to author in the bundled rich composer than ordinary prose.

## Related

- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-006 (@Mentions), FDR-030 (Inline Message Timestamps)
