---
name: chatto-formatting
description: Produce colored terminal-style artwork in Chatto messages. Use for ANSI art and colored text, not ordinary Markdown replies.
---

# Chatto ANSI artwork

Deliver artwork directly in an `ansi` code fence. Do not ask the user to run a
shell command. Chatto renders static art, not an interactive terminal.

Colors require an actual ESC character (U+001B) before each SGR sequence.
For cyan, use ESC followed by `[36m`, the text, and ESC followed by `[0m`.
Always reset styles. Bare `[36m`, literal `\033` or `\x1b`, and shell scripts
do not produce colored art. In JSON tool arguments, encode ESC as `\u001b`;
in assistant text, emit the actual character.

This example contains actual ESC characters:

```ansi
[36mcyan[0m
```

Keep each block within 80 columns and 100 rows. Only the first four ANSI blocks
in a message render. Keep borders and padding consistent across rows.
