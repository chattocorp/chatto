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
  leaves out a new pull request URL, the host appends the bare URL.
- Current exceptions are host fallbacks for when the model must not or cannot
  answer: the maintainer and research refusals and the duplicate-implementation
  refusal (a blocked tool must never be described as started work), the notice
  for a malformed model reply, and the reply failure message in
  `chatto/routing.ts`. Do not add new exceptions without discussion.

## Adding an implementation stage

Report a new stage, such as planning or review, as a milestone notice from the
implementation task, with its facts in `data`. Then name the milestone in the
supervisor's response policy (`workflows/response-policy.ts`) and in the
README. Do not add a host message template.
