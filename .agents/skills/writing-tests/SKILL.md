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

## Projection Refactors

A change to projection memory layout or storage must keep behavior equal to
the target branch. Use these checks in addition to unit tests:

- Replay a copied EVT store (`CHATTO_BENCH_EVT_STORE_DIR`) into the target
  branch and the changed branch. Compare the snapshot bytes of each changed
  projection.
- Restore the snapshot of the target branch in the changed branch. Snapshot
  the restored projection again and compare the bytes.
- Compare one digest of all read results over the real data, both live and
  after the restore. Put only values into the digest. Do not format pointers
  or protobuf messages with `%v`; marshal protobuf messages deterministically.
- Run the digest twice on the target branch before you compare. A different
  result shows a harness defect, not a regression.
- Call `CompleteStartupReplay` before you measure retained heap. Replay guards
  keep event IDs until replay completes.
- Compare timings in interleaved runs of the two branches. Another load, such
  as a test run, can change a single measurement.
- Keep the copied store in `.context/` and delete it after use. It contains
  real user data.
