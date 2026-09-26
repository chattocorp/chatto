---
name: chatto-audit-api
description: Audit Chatto API design for consistency, completeness, integration use, and future growth. Use for a whole-API review or a focused review of a service or workflow.
---

# Chatto API Design Audit

Ask: **Can an independent integration use Chatto's supported features without
copying the bundled frontend's assumptions?**

Review the public contract first. Inspect implementation only to confirm
behavior that the contract does not settle. Report implementation defects
separately from design gaps. This skill does not authorize fixes, issues,
commits, or PRs; those need a separate user instruction.

## Scope and Sources

- Follow root and path-specific `AGENTS.md`, including the current release
  policy. Keep Chatto separate from Authling and shared framework modules.
- Apply [API rules](../rules/chatto-api-rules/SKILL.md). Use
  [API compatibility](../chatto-api-compatibility/SKILL.md) for version and
  migration effects. Link to those rules instead of copying them.
- Start with public protobufs under `proto/chatto/`, mounted services, and
  generated API docs. Use [FDRs](../../../../docs/fdr/INDEX.md) to identify
  supported features and [interfaces](../../../../docs/architecture/interfaces.md)
  to check exposed surfaces. Use the [glossary](../../../../docs/GLOSSARY.md)
  for terms and relevant ADRs for intentional design choices.
- For a full audit, cover integration, admin, auth, discovery, and realtime
  contracts. Keep the local Operator API in a separate section if reviewed.
  For a focused request, limit the audit and state what was not checked.
- Check relevant callers, tests, and docs to confirm findings. The frontend
  shows one use of the API; it does not define the full requirement.

## Audit Method

1. Map services to resources and supported features. Note available reads,
   commands, collections, related IDs, and live events. Check that declared
   operations are mounted and usable by their intended callers.
2. Trace a few real workflows for the scope: a bot, an independent chat client,
   an admin tool, or a JSON-only integration. Start with discovery and access;
   follow the work through reads, writes, and refresh or reconnect as needed.
3. Apply the checks below. Mark each area as checked, not applicable, or not
   checked. Follow evidence; do not turn the checklist into a feature wish list.
4. Compare apparent gaps with documented product limits and design decisions.
   Confirm a concrete client problem before recommending a change.

## Design Checks

| Area | Questions |
| --- | --- |
| Names and ownership | Do service, method, field, and resource names have clear, consistent meanings? Are public, admin, self-only, and operator actions in the right scope? Are old aliases or duplicate operations still needed? |
| Resource shapes | Is each resource represented by a canonical type where access and lifecycle match? Do summaries clearly identify previews? Can related IDs be resolved without parsing labels, URLs, or private data? |
| Useful completeness | Can clients discover, read, change, and observe each supported feature? Can they read state they can write? Can they retrieve all members behind a count or preview? Are singular or batch reads needed for a real workflow? |
| Integration independence | Can clients work without a browser session, sidebar model, frontend cache, or large bootstrap response where these are not inherent requirements? Can bots use the features allowed by their permissions? Are defaults based on product meaning rather than one screen? |
| Updates and commands | Are masks, field presence, defaults, resets, nulls, and empty values clear? Are state updates distinct from commands? Does a successful write return useful resulting state or a clear way to read it? |
| Collections | Are potentially large results bounded? Are page defaults, limits, filters, counts, ordering, tie-breakers, and continuation rules defined? Do batch reads define limits, duplicates, result order, missing targets, and partial failures? |
| Access and privacy | Are caller identity, membership, permissions, and resource visibility clear? Are self-only fields kept private? Do counts, previews, batches, errors, and events respect the same visibility boundaries? |
| Errors and retries | Can clients distinguish bad input, missing access, missing resources, conflicts, limits, and temporary failure? Are no-op behavior and retry safety clear? What happens if a write succeeds but its response is lost? |
| Realtime and recovery | Can clients read current state, observe relevant changes, and recover after a gap? Can they resolve IDs from events? Are snapshot, event, ordering, cursor, and read-after-write contracts clear? Does live delivery complement explicit reads? |
| Future growth | Can supported features grow without unbounded responses or repeated breaking redesigns? Are enums, unions, optional fields, and version boundaries deliberate? Are extension points justified by real needs rather than hypothetical flexibility? |
| JSON and generated clients | Can both JSON and protobuf consumers express the same operations? Are masks, timestamps, integer counts, enums, and absence understandable? Are generated clients and public examples aligned with the contract? |
| Documentation | Can an API user identify required IDs, access, defaults, side effects, response meaning, and important limits without reading server code? Do guides agree with schemas and behavior? |

## Evidence and Verification

Use source paths and line references for findings. Distinguish a confirmed
problem from an open question. A missing test alone is not a design defect.

Run existing focused checks when they can resolve uncertainty. Schema lint
and compatibility checks do not prove usability or behavior. Do not run a full
suite or start a server merely to inventory the API. Follow repository browser
tooling rules when browser verification is needed. State what was verified.

Do not require CRUD, batch methods, events, or configuration for every resource.
Require them only when a supported workflow needs them. Treat documented
exceptions as choices to assess, not automatic violations.

## Report and Stop

Lead with a short verdict on integration use, completeness, consistency, and
future growth. State the scope and verification limits.

For each actionable finding, give:

- Priority and affected service or workflow.
- Source evidence and the concrete problem for an API consumer.
- The smallest useful change and why it improves the contract.
- Compatibility and caller migration effects; note stored-data effects separately.

Separate confirmed defects, design improvements, and open questions. Rank
findings by client impact, not naming preference. Group related findings into
small, coherent changes and recommend one next step.

Do not reopen explicitly deferred work unless new evidence changes its impact.
Do not invent follow-up tasks to keep the audit going. If no meaningful gaps
remain, say that the API is good enough for the reviewed scope and stop.
