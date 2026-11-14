---
name: chatto-formatting
description: Format Chatto replies with rendered SVG images, Mermaid diagrams, or ANSI art. Use before producing a visual, drawing, diagram, colored text art, or an explanation with a visual in Chatto.
---

# Chatto formatting

Post visuals directly in the current reply. Use a fenced code block whose opening
line has exactly `svg rendered`, `mermaid rendered`, or `ansi rendered` after the
three backticks. Plain language tags show source by default; they do not request
the rendered presentation. Do not add another modifier or wrap the visual fence
inside another code block. Use plain language tags only when the user asks for
source code instead of a displayed visual.

## SVG images

Use SVG for a drawing, illustrated comparison, or precise layout. Include the
SVG namespace and a `viewBox`. Keep shapes and labels inside the viewBox, leave
space around text, and use readable contrast. Use self-contained vector shapes
and text, without scripts, event handlers, external images, or external fonts.
Escape XML text such as `&` as `&amp;`. Prefer simple shapes over emoji whose
appearance and width depend on the device.

```svg rendered
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 120">
  <rect width="320" height="120" rx="12" fill="#172033"/>
  <rect x="20" y="20" width="180" height="80" rx="6" fill="#60a5fa"/>
  <rect x="220" y="40" width="80" height="60" rx="6" fill="#94a3b8"/>
  <text x="110" y="65" text-anchor="middle" fill="#111827">Selected view</text>
</svg>
```

Check that the drawing supports its labels. An enlarged item must actually be
larger than its peers. A before/after comparison must visibly show the relevant
change. Do not call an image rendered or inspected unless that was verified.

## Mermaid diagrams

Use Mermaid for workflows, relationships, sequences, and state transitions.
Quote labels that contain punctuation. Keep diagrams small enough for a chat
column. Do not include external links or interactive callbacks unless needed.

```mermaid rendered
flowchart LR
  A["Arrange view"] --> B["Navigate away"]
  B --> C["Return"]
  C --> D["Restore arrangement"]
```

## ANSI art

Use `ansi rendered` for static colored text art. The block must contain real ESC
characters (U+001B), followed by color sequences such as `[36m`. Reset with
ESC followed by `[0m`. In a JSON tool argument, encode ESC as `\u001b`; the decoded
message must contain the character, not the literal backslash sequence. Bare
`[36m`, shell commands, and text containing `\\u001b` do not produce colors.

Keep each block within 80 columns and 100 rows, with at most four ANSI blocks per
message. Align borders and labels. Do not use terminal cursor movement or commands
that require execution. Deliver the art itself, not a script for the user to run.
