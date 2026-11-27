---
name: chatto-code-review
description: High-level review rules for Chatto client and server changes, including API and protobuf changes, permissions, behavior regressions, multi-server compatibility, and multi-replica safety. Use for every review of a Chatto change, branch, or PR, including self-review.
---

# Chatto Code Review

Rules for reviews of Chatto changes in the backend and the frontend.

## API Changes

- Make sure that each API change is necessary.
- New server admin features use existing client API functions where possible.
  Add new client API functions when existing ones are not sufficient.
- Additive API changes are acceptable when they give value.
- Removed API fields or functions are breaking changes. Flag them as breaking.
- Accept a breaking API change only when its exact reason is known.

## Protocol Buffer Changes

- Use existing protocol buffer messages where possible. Add new fields or
  messages only when the change needs them.
- Follow the established patterns. Use submessages and enums where possible.
  Make sure that the change will not cause problems one year from now.
- For new sibling fields of the same message type, identify a valid response
  in which both fields are populated. If the contract rules that out, check
  whether one field with an enum expresses the distinction. See the
  [protobuf design rules](../../../proto/AGENTS.md#presence-and-api-shape).
- Removed protobuf fields or messages are breaking changes. Discuss each
  removal with the user.

## Chatto Permissions

- Add a new Chatto permission only when the feature requires it.
- Use the `{subject}.{verb}` format for permission names. Fit new names into
  the existing naming scheme.

## Behavior Regressions

- Compare changed defaults, limits, retry budgets, timeouts, and retention
  with the behavior on the target branch. Flag each change that the PR does
  not state and explain.

## Multi-Server Compatibility

- The change must not break the multi-server capabilities of the Chatto
  frontend.
- Increase the minimum connectable server version only when no other solution
  is possible. Discuss each increase with the user. Root `AGENTS.md` lists the
  evidence and PR labels that an increase requires. See
  [FDR-031](../../../docs/fdr/FDR-031-client-server-compatibility-discovery.md).
- Add new features to the server feature catalog
  (`ServerDiscoveryService.ListFeatures`, FDR-031). The frontend then can show
  them when the client and server versions differ.

## Multi-Replica Compatibility

- Each new and changed feature must work with one replica and with more than
  one replica.
- Replicas can run different versions during a rolling upgrade. The change
  must work in that condition. Follow the rules in
  [`cli/AGENTS.md`](../../../cli/AGENTS.md).

## Findings

- Confirm each finding in the code before you report it. Remove findings that
  you cannot support with a call path or a failure scenario.
