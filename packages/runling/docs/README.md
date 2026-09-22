# Runling documentation

Runling is an independent workflow and agent orchestrator. These documents belong
to Runling, including when applications such as ChattoBot use its APIs.

## Decision records

- [Architecture Decision Records](adr/INDEX.md) describe architectural choices,
  constraints, and consequences.
- [Feature Decision Records](fdr/INDEX.md) describe feature behavior, rationale,
  and open questions.

Both collections use independent numbering. Chatto and Authling records with the
same numbers describe different decisions. Root records apply to Runling only
when they explicitly describe a repository-wide decision.

The initial records document the current runtime, with emphasis on agents,
background tasks, and event sources. Their dates record when the decisions were
documented, not when each feature was first implemented. Experimental feature
records describe implemented behavior whose API can still change. Open questions
are not promises of future behavior.

## API and operation guides

- [Package README](../README.md): setup, CLI, run references, and releases.
- [Agent connections](agents.md): agent output, connections, task tools, and background tasks.
- [Agent steering](agent-steering.md): input delivery and consumption.
- [Task channels](task-channels.md): task input, output, and cancellation.
- [Event sources](event-sources.md): external inputs, retained state, and reloads.
- [Webhook routing](webhook-routing.md): HTTP delivery and run registration.
- [Workflow messages](workflow-messages.md) and [workflow text](workflow-text.md): interaction and output.
- [Timeouts](timeouts.md): deadlines and cancellation limits.
- [Server logs](server-logs.md): operational events.

Keep examples and API details in these guides. Update the relevant FDR when
behavior changes. Update or supersede the relevant ADR when an architectural
decision changes. Link FDRs to the ADRs that constrain them. Keep application
policy, such as a bot's permissions or conversation timeout, with the application.
