# Instructions for Agents Working in `apps/desktop/`

This file covers the experimental desktop application and its native helpers.

## Testing

- Native macOS helper behavior must have focused Swift tests wired into a macOS
  CI step. Desktop JavaScript checks and production helper builds do not compile
  Swift test targets.
