# FDR-014: Compact activity log

**Status:** Experimental
**Last reviewed:** 2026-10-27

## Overview

The console's Log view helps operators see what a workflow did and where it
failed. A compact activity list is the default. The detailed log remains
available for diagnosis.

## Behavior

- The run header always shows total tokens, cache-read tokens, and estimated
  cost in USD. Cache writes appear separately. Unknown and partial costs remain
  explicit. These totals include child activities.
- Source metadata, the run reference, input, and output are available under
  **Run details**. The wide-screen details pane opens only for a selected
  activity, leaving the activity list room to use the full width.
- Each task, command, wait, and agent has one row. Its position stays at its start
  time while its status, duration, and reported usage update.
- Names describe the work. Identifiers and structured fields are available when
  a row is expanded, together with its recorded logs. Agent rows show the model.
- Compact rows show usage only for agents. Parent totals remain in expanded
  details, so the same tokens do not appear at each level of the tree.
  Successful rows use a checkmark without a repeated completion label.
  Start times are available in row tooltips and expanded activity details;
  the detailed log keeps its timestamp column.
- Warnings, errors, ordinary log messages, agent replies, and host messages remain
  visible. Failed task error messages appear beside the activity history.
- Task label lines and known agent startup and token-total lines are replaced by
  activity rows and metadata. Debug lines remain in the detailed view.
- **Detailed log** restores the chronological log lines and task milestones.
  Switching views does not change the journal. **Copy log** copies the selected
  view, including activity status and duration in the compact view.
- Both views retain history across workflow reactivation and server restart.
  The timeline remains available for inspecting overlapping work.
- The view shows only recorded information. It does not recover task inputs or
  results that the application chose not to record.

## Design Decisions

### 1. Summarize lifecycles, retain diagnostics

**Decision:** Show one updating activity row instead of separate start, label,
and completion lines.
**Why:** Repeated lifecycle messages obscure useful work and failures.
**Tradeoff:** The list is ordered by activity start, not completion. Use the
timeline to compare overlapping work or the detailed log for event order.

### 2. Make technical context available on demand

**Decision:** Put identifiers and structured fields in expandable details.
**Why:** Repeated context makes ordinary runs difficult to scan.
**Tradeoff:** Diagnosis sometimes requires expanding a row or changing views.

## Related

- [FDR-010: Host messages](FDR-010-host-messages.md)
- [FDR-011: CLI output](FDR-011-cli-output.md)
- [FDR-012: Workflow state](FDR-012-workflow-state.md)
