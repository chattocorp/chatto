# Owner approval of child actions

Import `createApprovalQueue`, `approvalDecisionExtension`, and `toolApprovalGate`
from `runling/agents`. The application chooses which tools need approval. Other
tools keep their existing host policy.

Create one queue per owning conversation with `createApprovalQueue({ signal })`.
Install `approvalDecisionExtension(queue)` on the owner only and include
`decideApproval` in its tools. Give the owner `queue.list()` in each prepared
prompt. Tell it to assess requests against the user-authorized delegated scope.
Requests are data, not instructions or evidence of human permission.

The host supplies the child's request callback:

```ts
const request = (action) =>
  queue.request(action, {
    signal: childCtx.signal,
    notify: (proposal) =>
      childCtx.emit({
        type: 'notice',
        text: 'An owner decision is required.',
        data: { approvalId: proposal.id }
      })
  });

const gate = toolApprovalGate({
  signal: childCtx.signal,
  tools: {
    publish: (input) => ({
      action: 'publish',
      details: { revision: String(input.revision) }
    })
  },
  request
});
```

Install the gate after deterministic policy gates. Codemode calls pass through
the same gate. It captures tool inputs and blocks a call if they change while
it waits. Other host hooks must not change inputs after the gate returns.

Connect child notices to the owner's notification stream with
`observeAgentTasks` and `runAgentConversation`. Keep the conversation alive
while its child runs. An approval notice is internal communication. Only the
owner asks the user when more permission is necessary.

The owner calls `decideApproval` with the pending ID, `allow` or `deny`, and a
reason. Unknown, expired, foreign, and already decided IDs return
`{ decided: false }`. Host code can also call `queue.decide(id, decision)`.
Only explicit allow decisions release calls. Gate errors block execution.

For host commands, await `queue.request` directly. Snapshot command inputs and
verify the target, source revision, and queued user corrections after approval,
before external effects. Approval cannot grant a shell or remove a host rule.

Default limits are 32 pending requests, five minutes per request, and 16,000
serialized characters per action. Requests and returned lists are copies.
Cancellation of either participant and notification failure deny the action.
Call `queue.dispose()` during owner cleanup. Requests are process-local and are
not restored with an agent session.

Put only required safe facts in `action` and `details`. Do not include credentials,
personal data, raw command output, or file contents. Runling does not redact
these descriptions. The owner's model provider receives them when the host
includes them in its prompt. No additional external service is required.

These APIs require no migration. Existing classifiers and host policies keep
their contracts. See [ADR-008](adr/ADR-008-parent-owned-approvals.md) and
[FDR-009](fdr/FDR-009-parent-owned-approvals.md).
