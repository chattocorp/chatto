# ChattoBot instructions

These rules apply to `packages/chattobot/`. The root `AGENTS.md` rules also apply.

## Talking to users

- Only the supervisor task talks to the user. Other tasks, such as implementation
  and investigation, report to their parent task only (Runling ADR-006). Do not
  pass callbacks into a child task that post to Chatto.
- Do not hardcode user-facing message text in host code. The supervisor model
  writes every message in the user's language and its own words. A task reports
  facts: milestones and progress as `notice` updates with structured `data`, and
  its result. Text in notices and results describes the facts for the model; it
  is not shown to the user.
- Host code may guard facts, but not phrase messages. Example: when a reply
  leaves out a new pull request or issue URL, the host appends the bare URL.
- Current exceptions are host fallbacks for when the model must not or cannot
  answer: the maintainer and untrusted-content refusals and the duplicate-implementation
  refusal (a blocked tool must never be described as started work), the notice
  for a malformed model reply, and the reply failure message in
  `chatto/routing.ts`. Do not add new exceptions without discussion.

## Adding an implementation stage

Report a new stage, such as planning or review, as a milestone notice from the
implementation task, with its facts in `data`. When the supervisor must relay a
specific fact, such as a URL, add the milestone to `noticeReport` in
`workflows/task-context.ts`. Name the milestone in the README. Do not add a host
message template.

## Adding a way to call tools

Codemode lets the model call tools from a script. MCP and similar features add
other paths. Every path must keep these protections in place:

- The maintainer gate and the authorization checks in `workflows/chat.ts`. They
  read the latest message, so a path that can wait, such as a script, must not
  reach maintainer-only tools.
- Per-message budgets and limits, such as the research and attachment limits
  and the finding limit. Check that parallel calls cannot pass them.
- The block after untrusted content.
- The private temporary directory (`private-temp.ts`) for any output that
  is written to disk.

Have an adversarial review check each new path against this list before merge.
