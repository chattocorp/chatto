# Instructions for Agents Working in `apps/desktop/`

This file applies to the experimental desktop application and its native helpers.

## Testing

- Add focused Swift tests for native macOS helper behavior. Run the tests in a
  macOS CI step. JavaScript checks and production helper builds do not compile
  Swift test targets.
