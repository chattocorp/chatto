// Validates that the commit message GitHub will produce when this PR
// is squash-merged can be parsed by release-please's conventional-commits
// parser.
//
// Why this exists: release-please uses @conventional-commits/parser to
// drive version bumps and changelog entries. If the parser fails on a
// commit, that commit is SILENTLY dropped from both — no warning, no
// failed workflow, no missing version bump message. The release simply
// never happens (or skips the real feat/fix that should have triggered
// it). PR #595 hit this exact pattern: a `core.SpaceIDForKind(core.KindOfRoom(room))`
// inside a backtick code span tripped the parser, the alpha PR never got
// created, and we only noticed days later.
//
// See .claude/rules/commits.md for the rule that documents the failure
// mode this check guards against.

import { parser } from "@conventional-commits/parser";

const title = process.env.PR_TITLE ?? "";
const body = process.env.PR_BODY ?? "";
const num = process.env.PR_NUMBER ?? "";

if (!title) {
  console.error("::error::PR_TITLE is empty");
  process.exit(2);
}

// GitHub's squash-merge default builds the commit body as:
//   "<PR title> (#<number>)\n\n<PR body>"
// We construct the same string and feed it to the parser, so this check
// reflects what release-please will actually see after merge.
const message = `${title} (#${num})\n\n${body}`;

try {
  parser(message);
  console.log("OK — release-please will be able to parse this commit.");
} catch (err) {
  const annotation = [
    "release-please's conventional-commits parser cannot parse the",
    "commit body that GitHub will produce when this PR is squash-merged.",
    "",
    "Most common cause: nested parens inside `backtick code spans` in the",
    "PR description — e.g. `func(arg)` or `foo(bar.baz)`. The parser is",
    "markdown-unaware and treats every '(' as the opening of a scope token.",
    "",
    "Workaround: rephrase the offending span. Use a fenced code block,",
    "quoted prose, or drop the parens (e.g. 'the SpaceIDForKind helper'",
    "instead of `SpaceIDForKind(room)`).",
    "",
    "See .claude/rules/commits.md for the full rule.",
    "",
    `Parser error: ${err.message}`,
  ].join("%0A");

  // GitHub Actions workflow command — turns this into a red annotation
  // on the PR's Checks tab so the failure is visible without digging
  // through the job log. Multi-line annotations require URL-encoded
  // newlines (%0A), not literal '\n'.
  console.error(`::error title=PR body would break release-please::${annotation}`);
  process.exit(1);
}
