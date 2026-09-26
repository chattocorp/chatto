# Chatto coordinator demo

This proof of concept uses an agent to coordinate a planning conversation. The
root task defines schema-backed tasks and exposes them through agent-local
tools:

- `investigate`: create a read-only specialist agent and return repository findings.
- `ask_user`: call ordinary `input()` and return the Chatto user's answer.
- `implement`: show the complete plan, request approval, then run a coding task
  in an isolated worktree. Feedback returns to the coordinator for revision.

The coordinator chooses the order, asks questions, and discusses a plan. Its
prompt defaults to one focused investigation, reuses findings, and permits
parallel investigators only for independent questions that justify separate
agents. Each investigation shows its assigned question in the timeline. This is
prompt guidance, not a hard concurrency limit. The
specialist sees its assigned question; the coordinator keeps the conversation.
All agents record usage in the same workflow context. Child tasks appear in
the Runling timeline. The `taskTool()` adapter wraps explicit task calls as agent
tools; spawning and consuming child updates remain coordinator decisions.

## Try it

Use the same `packages/runling/.env` credentials, required `CHATTO_WORKING_COPY`,
and model setup as the [planning demo](chatto-plan-demo.md).
Set the bot's outbound webhook to:

```text
http://localhost:5173/api/runs/start/chatto-coordinator-demo
```

From the monorepo root, start `mise x -- pnpm --dir packages/runling dev`
and send a new root DM, such as “Help me plan a Darcula theme.”
Send another thought while it works, and answer its questions in the reply thread.
Use only this webhook for that bot while testing, to avoid running both demos.

The demo investigates `CHATTO_WORKING_COPY` without updating or editing it.
When the implementation tool displays a plan, reply with exactly `/implement`
to approve that plan, or send feedback to revise it. Approval is checked in code;
the coordinator cannot grant it. A busy-chat command does not skip this prompt.
The existing planning and non-agentic input demos remain available for comparison.

## Implementation

After approval, the task requires a clean source checkout and creates a branch
and worktree from its current HEAD under `.context/chatto-implement/` in Runling's
working directory. It installs dependencies with `pnpm install --frozen-lockfile`.
It does not fetch the source checkout. The thread receives status messages for
setup, coding (including the worktree path), validation, and each repair. Typing
continues while the task works; the coordinator posts the final result.

The coding agent receives the approved plan and explicit worktree directory.
It adds tests and runs checks relevant to the changed components. The task then
runs `pnpm run check` and `pnpm test`, feeding failures back to the agent for up
to three validation attempts in total. Each setup/check/test command has a
10-minute limit. These root pnpm scripts do not replace component-specific
backend or browser verification.

The worker is instructed to leave changes for review without commits, pushes,
merges, or deployment. The tool returns the branch, directory, and implementation
summary. Worktrees remain after success, failure, or cancellation; inspect them
before removing them. One conversation can attempt implementation once. Start a
new DM after inspecting a failed attempt.

## Boundaries

The coordinator uses `spawn()` for investigations and implementation. Each child
uses [`connectAgent`](../docs/agents.md) from `runling/agents` to forward `ctx.inbox`
messages and sends typed updates with `ctx.emit()`.
The coordinator reads `child.updates`, posts progress to Chatto, and awaits
`child.result`.

Every busy message goes to the coordinator. When exactly one specialist task is
active, it also receives a queued copy through `child.send()`. The timeline shows
queue acceptance, task reads, and agent consumption separately. Sending
to the queue alone does not confirm consumption. Messages received during setup
or validation, or while several children are active, remain available to the
coordinator. `/cancel` reaches the children through their shared cancellation
signal. Once a question opens, the next new message answers it. Questions allow
15 minutes, and concurrent questions are presented sequentially by the adapter.
Typing refreshes pause while questions wait for input.

The run finishes when the coordinator reports its final result. Messages that miss
its interaction's steering window start another interaction before the final
reply. After the run ends, start a new root DM. Conversation state is in memory,
and the same local-only webhook setup limits as the planning demo apply.

The coordinator prompt asks it to put greetings and questions together in
`ask_user`, without a separate assistant-text preface. There is no code-level
suppression of its conversational messages.

Specialist agents send brief progress updates through `ctx.emit({ type: "text", text })` to the same
Chatto thread. Task startup and delivery acknowledgements appear in the timeline. Investigations
announce completion in the thread. Complete
specialist results go to the coordinator. Intermediate messages are delivered
after each assistant message completes; private reasoning is not forwarded.

Each specialist returns a complete report. If steering produces another report
within the same interaction, the returned details retain earlier findings and
label the latest report as authoritative. Approval timeouts tell the coordinator
that implementation did not start and no worktree was created.

## Code structure

The root workflow defines the agent tools and owns child tracking, steering,
and the final inbox drain. `runling/agents` provides `taskTool()` for tool signal
composition and text results.

The `chatto/` helpers keep separate responsibilities:

- `investigate.ts`: read-only research and its agent lifetime.
- `approve-implementation.ts`: plan approval and the one-attempt guard per conversation.
- `implement.ts`: worktree setup, implementation, and validation.
- `conversation.ts`: ordered replies and typing suppression while questions are pending.

These extractions preserve the existing message routing and approval flow.
