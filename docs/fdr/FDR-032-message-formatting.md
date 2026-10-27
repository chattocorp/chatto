# FDR-032: Message Formatting

**Status:** Active
**Last reviewed:** 2026-10-27

## Overview

Message bodies are stored and exchanged as plain text while bundled clients render a deliberately limited Markdown subset. This gives people useful structure for chat, code, and compact tabular data without making message content depend on a client-specific rich-text document format.

## Behavior

- Automatic links require `www.` or an allowed protocol: `https://`, `http://`, or `mailto:`. Bare domains, filenames, email addresses without `mailto:`, and protocol-relative URLs stay plain text. Explicit Markdown links also allow root-relative paths that start with one `/`, such as `[room](/chat/example.com/R8rLgzhPkYja7Gs)`. These paths do not produce link previews, and the visual editor preserves them. Other protocols stay unlinked. This rule prevents filenames such as `agents.md` and `foo.py` from becoming links. The source editor uses the same prefix rule for URL highlighting. Selecting a `mailto:` link opens the configured mail application.
- Messages support paragraphs, ATX headings, emphasis, links and autolinks, inline code, fenced and indented code blocks, blockquotes, and ordered and unordered lists.
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
- Rendered code blocks show subtle line numbers without a background in Source mode. The numbers scroll with the source and are excluded from copied source. Each block has a header with its language and a button to copy its code without the Markdown fence or display-only tab spacing. SVG, Mermaid, and ANSI blocks add Source and Render tabs. Plain language hints select Source; the exact optional `rendered` hint selects Render. Tab state belongs to the mounted block, does not change the message, and resets when its HTML changes. Tabs support arrow keys, Home and End. Render failures select Source, disable Render and show a localised error.
- Source code contents have a maximum height of 24 rem and scroll in both directions when needed. The header stays outside the scroll area. This also applies to ordinary code blocks.
- SVG and Mermaid previews reuse the image viewer's pan/zoom component. The inline stage fills the frame width and is at most 30 rem or 60% of the viewport height, whichever is smaller. It starts with the complete image fitted and centered. Scroll or drag to pan after zooming; pinch, Ctrl-wheel, or the zoom buttons to zoom. Fit restores the complete image. Shared edge gradients indicate hidden content. Wheel input at a boundary continues through the conversation. Source/Render switches preserve the canvas view.
- Fenced blocks labelled `svg` offer an image in the Render tab. Invalid images reveal their source. Each message can show up to four SVG previews, each with at most 100,000 source characters. Larger or additional blocks show source. Previews fit the message width and preserve transparency. Scripts, interactive links, and external resources are disabled. Self-contained SVG animations can play.
- Fenced blocks labelled `ansi` offer static ANSI art in the Render tab. They support terminal colors, Unicode text, cursor movement, and erase commands. Each block uses an 80-column, 100-row screen with no scrollback. Unused bottom rows are hidden. Wide art scrolls horizontally; all screen rows remain visible. The first four ANSI blocks in a message render as art; additional blocks show their source. Copy retains the escape codes. Artwork uses a fixed black background in both themes. It cannot blink, open terminal links, access the clipboard, or make external connections.
- The message menu offers **Copy message text** to copy the full Markdown source. When text is selected wholly within that message body, **Copy selected text** appears immediately above it and copies only the selected text. A selection outside the message or across messages does not show this action. The menu keeps the selected text when it opens, even if focus changes.
- Right-clicking a link in a message body opens the message menu. **Copy Link** copies that link's URL; **Copy Message Link** copies the message permalink.
- The app applies the editor choice to every registered Chatto server. It does not sync this App Preference to other browsers or devices.
- The app applies the sending keys to every registered Chatto server. Return sends by default. People can select the platform modifier plus Return instead.
- The key not assigned to sending performs the selected editor's normal Return action. In the visual editor that includes paragraph splitting, list continuation, leaving an empty list item, and new lines inside code blocks; Shift+Return remains a hard line break.
- Both editors provide toolbar actions to indent and outdent. In the visual editor they change list nesting. In the Markdown source editor they change the indentation of the current line or selected lines.
- In the Markdown source editor, Tab and Shift+Tab move keyboard focus without changing the text. Autocomplete still handles these keys first: Tab selects a suggestion or completes a mention, and repeated Tab presses cycle through matching mentions. An Escape prefix is not required for normal focus navigation.
- The composer shows message actions below the text in narrow panes. In wider panes, actions share the input row until the draft grows beyond one line. They then stay below the text until the draft is cleared or sent, or its destination or edit mode changes. This prevents the layout from switching repeatedly when the extra width removes a line wrap.
- A Formatting options control shows or hides the formatting toolbar above the input. The toolbar is hidden by default. The app stores the last selection in the browser and applies it to all composers. Both toolbars use larger buttons and icons when touch input is available, including on hybrid devices. The formatting toolbar scrolls horizontally when its controls do not fit.
- Touch-primary devices always use Return for editing and the visible Send button, even when Return-to-send is selected.

## Design Decisions

Code blocks in both composer editors use the same frame colors and thin header as
message code blocks. Editors do not show line numbers. The visual editor places
its language selector and Render by default toggle in the header. The Markdown
editor keeps the fence text editable.

Scrollable message code blocks use the shared edge gradients. Fades show only
where more content is available, including horizontal ANSI output. They follow
the message background and stay below the code header.

### 1. Plain-text Markdown is the interchange format

**Decision:** Message formatting is represented by Markdown in the existing plain-text body rather than by a client-specific rich-text document.
**Why:** Plain text remains portable across API clients, server versions, exports, and clients that implement only a subset of formatting. Unsupported syntax can still be displayed instead of making the message unreadable.
**Tradeoff:** Clients must implement compatible rendering themselves, and a rich composer cannot represent every valid source construct as a dedicated editing control.

### 2. The supported syntax is deliberately constrained

**Decision:** The bundled renderer enables common conversational structure, GFM tables, and named footnotes while keeping source HTML, images, and several lower-value block constructs disabled.
**Why:** A small reviewed output surface is easier to keep predictable and safe in user-authored messages. File attachments provide the general path for images; explicit SVG previews use the bounded image behavior below.
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

### 6. SVG previews use an isolated image

**Decision:** Offer SVG images through the shared artifact tabs. Create an image only when Render is selected and the block approaches the viewport. Show source if the image fails to load. Load the artwork in the browser's restricted image context.
**Why:** Bots can send diagrams through the existing message body while people can still share ordinary SVG code blocks. Image mode prevents scripts and external resource loads, including connections that would reveal the viewer's IP address.
**Tradeoff:** SVG documents must be self-contained and include the SVG namespace. Other clients can show source. The preview is not an interactive document.

### 7. Mermaid diagrams use Pele and the image boundary

**Decision:** Recognise `mermaid` fences with an optional `rendered` hint, ignoring case. Bundle Pele locally and load its worker only when Render is selected and the diagram approaches the viewport. Display the generated SVG as an image in the shared frame. A loading result must not change the selected tab. Show source on failure. The composer, stored message and message copy retain the source. Use `accTitle` and `accDescr` as image text; without them, use the source as the text alternative.

Disable links and external images. Reject frontmatter and configuration directives. Do not supply icon or math markup resolvers. No diagram content is sent to an external service. Apply the current application colours inside each image and update them when the theme changes. Default node shapes use the shared shell-lighting values for UI Depth: a surface gradient and inset bevel, with no outer shadow. Flat mode removes this treatment. Depth changes update the image without running layout again. Author-specified fills remain unchanged.

Allow at most four diagrams per message and 10,000 source characters per diagram. Limit parsed model objects to 1,000, flowchart edges to 250, SVG output to 500,000 characters, each dimension to 8,192 pixels, and area to 16 million pixels. Reuse one worker for sequential jobs and queue at most 32 jobs. Allow up to 30 seconds to load and start the worker; each diagram then has a separate three-second execution limit. Discard the worker after a timeout, worker error or active-job cancellation; the next job starts a new one. An HTML update or unmount cancels its jobs and disposes its mounted image canvases.

Default flowchart groups use the menu frame treatment: a solid surface, a title band, and an inset content area. Frame lighting follows UI Depth and the current theme. Groups with author-specified styles keep their original presentation.

Both editors highlight labelled Mermaid source with a shared, lazily loaded grammar. The visual editor includes Mermaid in its language selector. Mermaid is excluded from automatic language detection so a guess cannot enable diagram rendering. Highlighting does not validate the diagram; Pele validates it when rendering. The visual editor offers a Render by default toggle for SVG, Mermaid and ANSI blocks. It writes or removes the rendered hint without changing source. Changing between artifact languages preserves the setting; choosing another language removes it.

**Why:** Pele supports Mermaid syntax without a remote rendering service. The image boundary also used for SVG prevents active content from entering the application document. Worker termination bounds expensive layout work.

**Tradeoff:** Unsupported, invalid or excessive diagrams remain code. Pele can differ from Mermaid.js. Cached SVG belongs to the mounted HTML version, with at most four entries; theme changes reuse it, but remounting renders again. This keeps private message content out of a global cache. The Markdown hints are provisional until the message presentation interface is reviewed.

## Related

- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-006 (@Mentions), FDR-030 (Inline Message Timestamps)
- **Guide:** [Format Messages](../../apps/docs-website/src/content/docs/getting-started/message-formatting.mdx)
- **Issue:** [#2007](https://github.com/chattocorp/chatto/issues/2007)
