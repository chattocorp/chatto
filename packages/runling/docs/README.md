# Runling documentation

Runling is an independent workflow and agent orchestrator. These documents belong
to Runling, including when applications such as ChattoBot use its APIs.

## Decision records

- [Architecture Decision Records](adr/INDEX.md) describe architectural choices,
  constraints, and consequences.
- [Feature Decision Records](fdr/INDEX.md) describe feature behavior, rationale,
  and open questions.

For terminal behavior, [FDR-011](fdr/FDR-011-cli-output.md) defines the CLI
contract: live run details, one final root result, spacing, agent prefixes,
and JSON output. Read it before changing terminal output.

Both collections use independent numbering. Chatto and Authling records with the
same numbers describe different decisions. Root records apply to Runling only
when they explicitly describe a repository-wide decision.

The initial records document the runtime as it was when they were written.
Their dates record when the decisions were documented, not when each feature
was first implemented. [ADR-009](adr/ADR-009-task-invocations.md) made typed
task invocations the execution model and removed the agent supervision layer.
Records about removed features are marked withdrawn or retired. Experimental
feature records describe implemented behavior whose API can still change. Open
questions are not promises of future behavior.

## API and operation guides

- [Package README](../README.md): setup, CLI, tasks, run references, and releases.
- [Task invocations and messages](task-channels.md): `ctx.start`, message
  schemas, `ctx.steering`, `ctx.emit`, background children, and cancellation.
- [Workflow state](workflow-state.md): mutable root state, periodic snapshots,
  configuration keys, and root-owned restart recovery.
- [Workflow messages and output](workflow-messages.md): messages between the
  host and a run, and the output of a run.
- [Agents](agents.md): output modes, classifiers, session stores, private temporary
  files, trust, codemode, and task tools.
- [Agent steering](agent-steering.md): messages to a running agent.
- [Event sources](event-sources.md): external inputs, retained state, and reloads.
- [Webhook routing](webhook-routing.md): HTTP delivery, run handles, and run registration.
- [Timeouts](timeouts.md): deadlines and cancellation.
- [Server logs](server-logs.md): operational events.

Keep examples and API details in these guides. Update the relevant FDR when
behavior changes. Update or supersede the relevant ADR when an architectural
decision changes. Link FDRs to the ADRs that constrain them. Keep application
policy, such as a bot's permissions or conversation timeout, with the application.
