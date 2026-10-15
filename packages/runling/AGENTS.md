# Runling instructions

These instructions apply to `packages/runling/`. The repository's root
`AGENTS.md` also applies while Runling is in this repository.

Runling is an independent workflow and agent orchestrator. Keep its public
APIs and runtime independent of Chatto and Authling. Application permissions,
such as who can request a GitHub change, belong to the application.

## Read before changing behavior

- Start with the [documentation index](docs/README.md).
- Read the relevant [ADRs](docs/adr/INDEX.md) for architectural constraints
  and [FDRs](docs/fdr/INDEX.md) for feature requirements and their rationale.
- For tasks and agents, read
  [ADR-009](docs/adr/ADR-009-task-invocations.md) and
  [FDR-001](docs/fdr/FDR-001-agent-interactions.md).
- For terminal output, read [FDR-011](docs/fdr/FDR-011-cli-output.md).
  Preserve both live run details and the separate final root result.
- Use the linked API guides for examples and usage details. Check the source
  and relevant tests before describing behavior as implemented.

## Keep decisions current

- Update the relevant FDR when behavior or its rationale changes. Use an ADR
  for a change to the architecture. Keep examples and API details in guides.
- Keep records in `docs/adr/` and `docs/fdr/`, with Runling's own numbering.
  Update the corresponding index and links when adding or replacing a record.
- Rewrite obsolete requirements in place. Record proposals and unresolved
  limits as open questions, not implemented behavior.
- Keep requirements in one primary record and link to it from related records.
  Include the reason for each non-obvious decision so later agents can preserve it.
- Keep relevant tests current and run checks that can find regressions. For
  CLI changes, verify the compiled executable as well as the runtime behavior.
- Document required public API migrations in the relevant guide. Keep
  application-specific policies in the application's documentation.

See the [package README](README.md) for development and verification commands.
