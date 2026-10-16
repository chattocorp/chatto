---
name: chatto-frontend-review
description: Review Chatto frontend code for side-effect ownership, Svelte reactivity, lifecycle behavior, and unnecessary complexity. Use for every review that includes apps/frontend, including self-review, branch or PR review, and frontend architecture review.
---

# Chatto Frontend Review

Review the requested scope, including uncommitted changes when applicable.
Read [frontend instructions](../../../apps/frontend/AGENTS.md) and trace
changed behavior through its callers. Apply these rules before declaring the
review complete. Passing tests or the Svelte analyser do not replace this review.

## Rules

1. **Use existing event handlers.** Before adding an effect or lifecycle hook,
   check whether an existing handler can perform the action. For example, handle
   a new notification in the notification-update handler, rather than watching
   notification state with `$effect`.
2. **Derive values; do not synchronize copies.** Use `$derived` for computed
   state. Do not use `$effect` to copy state, calculate values, or initiate a
   business operation that belongs in an event handler.
3. **Avoid new `$effect` calls.** Require a concrete need to synchronize with
   an external system and evidence that existing handlers cannot own the work.
   Check dependencies, repeated execution, and cleanup.
4. **Avoid `untrack`.** Do not use it to hide dependencies, stop a feedback
   loop, or make an effect run only once. Fix the data flow. An intentional
   snapshot must explain why later changes must not affect its consumer.
5. **Avoid mount and unmount hooks for business behavior.** `onMount`,
   `onDestroy`, and effect cleanup are for resources whose lifetime belongs to
   the component. Do not use them to mark data read, save changes, or trigger
   requests merely because a component appears or disappears. Prefer existing
   owners for subscriptions and timers; use Svelte window/document handlers
   for browser events.
6. **Do not move the same problem into a wrapper.** An attachment, headless
   component, hook, reactive class, or keyed remount is not a valid fix merely
   because it hides `$effect`, `untrack`, or `onMount`. The actual trigger and
   owner must be correct. Genuine DOM/library setup and cleanup are valid
   lifecycle work; explain why the chosen lifetime is required.
7. **Keep state in its proper layer.** Apply
   [chatto-client-placement](../chatto-client-placement/SKILL.md). Avoid copied
   server state and extra flags or collections when existing state suffices.
   Keep rendering, derived expressions, and template helpers free of mutations.
8. **Check failures and transitions.** Verify the relevant entry, navigation,
   focus, permission, retry, and disposal paths. Look for duplicate requests,
   stale completions, retry loops, and missing cleanup. Tests must exercise the
   actual trigger and integration, not only a helper called directly by a test.
9. **Follow the design system.** User-facing changes must fit the existing
   design system. Reuse shared Tailwind utilities and Svelte components where
   possible. If the change needs a new utility, component, or design-system
   rule, tell the user what is needed and why.
10. **Look for nearby simplifications.** Review the changed code and the code
    it works with for duplication, unnecessary state, and avoidable complexity.
    Suggest concrete simplifications that help the current change. Keep broader
    refactors as separate suggestions.

## Findings

For each finding, give the file and line, the rule, the concrete consequence,
and the simpler existing integration point when one exists. Review each new or
changed `$effect`, `untrack`, and lifecycle hook explicitly; report unjustified
uses as findings. Do not demand changes to unrelated existing uses.

Report findings in severity order, followed by checks run and remaining gaps.
If there are no findings, state the reviewed scope. For review-only requests,
do not edit files. This skill does not require or authorize subagents.
