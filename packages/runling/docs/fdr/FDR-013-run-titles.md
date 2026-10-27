# FDR-013: Workflow run titles

**Status:** Experimental
**Last reviewed:** 2026-10-27

## Overview

Workflows can set a descriptive display title so operators can distinguish runs
of the same task. Titles are optional; unnamed runs keep the workflow type as
their heading.

## Behavior

- A title belongs to the workflow run. It does not change its stable reference,
  task names, routing, or execution status.
- Workflow code can read and update the title during execution. Updates appear
  live in the console list and inspector and persist through journal replay,
  restart, and reactivation. The inspector still shows the workflow type.
- Titles contain 1–120 Unicode characters after whitespace normalization.
  Empty titles, other control characters, and oversized values are rejected.
  Repeating the current title does not create another update.
- Titles render as plain text, not Markdown or HTML. They appear in shared run
  lists and history; applications must exclude secrets and personal details.
- Hosts that do not provide a run identity have no title API. Cancelled or ended
  workflow executions cannot change a title through a retained setter.
- Persistence uses the existing journal contract. This is display metadata, not
  an acknowledgement of application work. Old journals need no migration.

## Design decisions

Runling provides a generic metadata operation. Applications choose whether a
title comes from deterministic code, a description task, or an agent tool. Setting
a title does not itself make a model request. Runling has no automatic naming
policy or conversation-specific primitive. Applications can use
[`createDescription`](../agents.md#describe-supplied-content) to produce descriptions.

## Related

- [ADR-012: Router-controlled workflow reactivation](../adr/ADR-012-router-controlled-reactivation.md)
- [FDR-012: Workflow state and restart recovery](FDR-012-workflow-state.md)
- [Workflow state guide](../workflow-state.md)
