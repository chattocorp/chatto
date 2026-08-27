---
name: "writing-tests"
description: "Instructions and hints for writing tests."
---

## Testing Judgment

- Pick the lowest test layer that exercises the change, but do not stop below
  the layer where the bug could occur.
- When testing an early rejection, use input that would fail a later check. The
  test should still return the early error.
- Choose additional integration or end-to-end coverage when the regression can
  occur only across component or process boundaries.
