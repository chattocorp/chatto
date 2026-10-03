# ADR-008: Parents own approval of delegated actions

**Status:** Accepted
**Date:** 2026-10-03

## Context

A child can propose an action that exceeds its standing permission. Its parent
knows the delegated goal and the user's request. The child must not approve
its own action or contact the user without its parent.

The classifier in [ADR-007](ADR-007-authorization-classifier.md) checks selected
human messages. It does not provide a channel for a child to wait for its owner.

## Decision

The host can create an approval queue for one owner. A child requests approval
through a host callback and sends a notice to its parent. The parent sees the
pending requests and can allow or deny one exact action.

Each request has a random, single-use ID. A decision belongs to that owner's
queue and releases only that waiting action. Requests expire and are denied
when either participant stops. Queues do not survive restart.

Applications choose standing permissions and the actions that need approval.
Approval cannot add tools or override host restrictions. A tool gate checks
that its inputs did not change while it waited. A host that approves commands
must also verify the target and source revision before execution. Request
descriptions are data, not authority. Only the owner can ask its user for more
permission. This extends the ownership rule in
[ADR-006](ADR-006-parent-child-communication.md).

## Consequences

The parent can use its conversation context to assess a request. The child
waits without blocking the parent's conversation. Routine operations can retain
standing permission. Each approval costs an owner turn.

The host must bound requests, exclude secrets and personal data from descriptions,
and connect cancellation to the queue. Existing authorization and tool
restrictions remain necessary. A parent model can make a wrong decision;
approval does not make it a security boundary.
