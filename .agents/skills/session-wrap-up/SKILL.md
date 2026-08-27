---
name: session-wrap-up
description: Review a completed, paused, or blocked coding-agent session to identify friction, mistakes, near misses, missing context, and opportunities to improve future work. Use when the user asks to wrap up, run a retrospective, explain what issues the agent faced, capture lessons learned, or recommend improvements to repository instructions, documentation, skills, tooling, tests, or workflows. Produce an evidence-based, propose-only report; do not apply follow-up changes unless the user asks.
---

# Session Wrap-Up

Run a candid, compact retrospective of the current session. Base it on the
conversation, commands, tool output, edits, verification, and unresolved work;
do not invent friction to make the report look substantial.

## Interrogate The Session

Ask yourself:

- Where did I lose time, take a false start, or repeat work?
- What did I misunderstand or assume too early?
- What did the user's questions or corrections reveal about unclear scope,
  location, status, or explanation?
- Which instructions were missing, hard to discover, ambiguous, or in tension?
- Which code, documentation, architecture, or terminology did I have to
  rediscover?
- Which tool, dependency, environment, build, or test failure slowed the work?
- What nearly went wrong, even if the final result was correct?
- What remains incomplete, uncertain, or insufficiently verified?
- What worked especially well and is worth preserving or standardising?
- What single change would most improve a similar session next time?

Verify the factual premise behind user questions and corrections against the
available evidence. Treat a correction as strong evidence of an expectation or
communication gap even when its diagnosis is not confirmed.

Before reporting the outcome, perform only the cheap read-only checks needed to
confirm the current state. For repository work, normally inspect working-tree
status and the changed-file summary, then reconcile them with verification
already recorded in the session. Do not rerun expensive tests solely for the
wrap-up. State when relevant evidence is unavailable.

Distinguish observed facts from diagnosis and speculation. Say "No material
friction observed" when that is the honest conclusion.

## Classify Findings

Assign each meaningful issue to the narrowest useful category:

- **Agent practice**: planning, assumptions, sequencing, communication, tool
  choice, or verification discipline.
- **Repository knowledge**: `AGENTS.md`, documentation, architecture inventory,
  glossary, examples, or code discoverability.
- **Reusable workflow**: a missing or weak skill, script, task, template, check,
  or automation.
- **Environment or tooling**: dependencies, permissions, local setup, external
  services, or product limitations.
- **Task definition**: unclear scope, acceptance criteria, ownership, or product
  boundary.

Keep Chatto, Authling, shared-framework, and repository-wide recommendations
separate. Place a proposed improvement with the product or infrastructure that
owns it.

## Recommend Improvements

For each material finding, propose the smallest change likely to prevent the
same class of friction. Include:

- the evidence and impact;
- whether the root cause is known or inferred;
- the concrete improvement and likely owner or location;
- the expected payoff and any trade-off;
- a priority of **now**, **soon**, or **only if repeated**.

Prefer durable fixes over reminders. Examples include tightening a scoped
instruction, adding a focused test, exposing an existing task, improving an
error message, documenting a hidden invariant, or extending a relevant skill.
Do not recommend new process or documentation when a simpler code or tooling
fix would remove the problem.

Keep the wrap-up propose-only. Do not edit files, create issues, send feedback,
or start follow-up work unless the user explicitly asks.

## Report Format

Return one concise report with these sections:

### Outcome

State the session goal, result, and verification level. Call out incomplete or
unverified work without repeating the entire implementation handoff.

### Friction And Near Misses

List only evidence-backed findings. For each, give the category, what happened,
and its practical impact. Separate agent mistakes from repository or tooling
problems. Treat direct user corrections as high-signal evidence of an
expectation or communication gap, not automatic proof of their diagnosis. Do
not infer dissatisfaction from tone alone.

### What Worked

Capture practices, instructions, tools, or repository affordances that made the
session more reliable or efficient.

### Improvements

Prioritise at most five concrete proposals. Name the likely owner or file when
known, and mark inferred diagnoses clearly.

### Recommended Next Step

Choose the single highest-leverage action. If no follow-up is warranted, say
so. End with one short question only when the user's perspective would
materially change the recommendation.

## Guardrails

- Be specific and candid without assigning blame or performing false humility.
- Do not exaggerate routine exploration into a systemic problem.
- Do not hide the agent's own errors behind tooling or instruction complaints.
- Do not expose secrets, personal data, or sensitive command output.
- Do not claim that a proposed improvement is validated when it has not been
  tried.
