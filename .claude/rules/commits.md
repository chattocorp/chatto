# Making Git Commits

- Use conventional commits with a clear, descriptive message.
- Use the commit description for a bullet list of changes made.
- **Avoid `parens(in)` inside backtick code spans** in commit bodies (and squash-merge PR descriptions, which become the body). release-please's conventional-commits parser is markdown-unaware: a `` `func(...)` `` or `` `foo(bar)` `` inside a code span trips its state machine and the whole commit is silently dropped from version-bump consideration _and_ the changelog (real example: PR #582 was missed because its body had `` `time.AfterFunc(5*time.Second, func() { ... })` ``). Rephrase to use a quoted string, a fenced code block, or natural prose without nested parens — anything that doesn't put `(` … `)` between backticks.

# Creating Pull Requests

- Use Conventional Commit formatting for the PR title. Keep in mind that release-please makes version bump decisions based on this, so set breaking changes where adequate (and avoid otherwise.)
- In the PR body, include a concise bullet-point summary of the changes and their motivation. If the PR addresses a GitHub issue, link it in the description (e.g., "Fixes #123").
- When a PR implements a GitHub issue, the PR body MUST include a GitHub closing keyword in its own paragraph, e.g. `Closes #604.` Prefer `Closes #123` over a plain issue link so merges can auto-close the issue.
- If the user provided an issue URL or number anywhere in the task, carry that issue number through to the PR body before creating the PR.
- After creating or editing the PR, verify issue closure wiring with `gh pr view <pr> --json body,baseRefName,closingIssuesReferences`.
- If `closingIssuesReferences` is empty, do not assume the issue will close automatically. Fix the PR body if the closing keyword is missing. If the keyword is present but GitHub still does not report a closing issue, tell the user explicitly, especially when the PR base is not the repo default branch.
- When creating or editing a multiline PR or issue body with `gh`, write the body to a markdown file and use `--body-file`. Do not pass escaped `\n` sequences to `--body`; they render literally on GitHub.
- After creating a PR, always check that CI passes. If CI fails, proactively diagnose and fix the failures without waiting to be asked.
- **The baseline for test failures is ALWAYS `main`, never the previous commit on the branch.** If a test passes on `main` but fails on your branch, it is a regression you introduced and you MUST fix it. Do not dismiss a failure just because a previous commit on the same branch also had it. The only tests you may ignore are those that are also failing or flaky on `main`.
- Common CI failure sources: broken tests from removed code paths, nil loggers in test setup, ESLint missing keys in Svelte `{#each}` blocks, and test selectors that are too broad.

# Merging Pull Requests

- Before merging a PR, first merge `origin/main` into the branch to ensure it's up-to-date.
- Run tests after merging to catch any integration issues before the final merge.

# Working with GitHub Issues

This repo uses GitHub's modern issue features. Prefer them over hand-rolled checklists/labels.

## Issue types (GA 2025)

- The `chattocorp` org has **Task**, **Bug**, and **Feature** types enabled. Always set a type when creating issues.
- Use **Feature** for hub/epic issues that group a body of work; use **Task** for individual sub-issues; use **Bug** for defects.
- Set the type at creation via `gh issue create --label` is NOT how this works — the type is a separate field. Use `gh api graphql` with the `updateIssueIssueType` mutation, or pass `issueType` to `createIssue`. Look up the type ID once via:

  ```sh
  gh api graphql -f query='query { repository(owner: "chattocorp", name: "chatto") { issueTypes(first: 20) { nodes { id name } } } }'
  ```

## Sub-issues (GA 2025)

- Use parent/child sub-issue relationships for any multi-PR effort. The parent issue gets a native progress bar driven by closed sub-issues — no manual checklist sync.
- When creating a child issue for an epic, set GitHub's native parent/sub-issue metadata immediately. Do not rely on body text like `Parent: #123` as the only link.
- Prefer creating sub-issues through GraphQL `createIssue` with both `issueTypeId` and `parentIssueId` set. If the issue already exists, link it via `addSubIssue`. `subIssueId` is the GraphQL node ID, not the issue number.
- After creating or linking sub-issues, verify the native relationship with a GraphQL query before reporting the work as done:

  ```sh
  gh api graphql -f query='query($owner:String!,$repo:String!,$number:Int!){ repository(owner:$owner,name:$repo){ issue(number:$number){ number subIssues(first:50){ nodes { number title parent { number } } } } } }' -F owner=chattocorp -F repo=chatto -F number=123
  ```

- Sub-issues can span repos in the same org. Don't bundle unrelated work under one parent just because they share a theme — keep parents tight.
- For epics, write the hub issue body to capture the _why_ (motivation, key decisions, phase breakdown). Don't duplicate the per-sub-issue scope into the hub — the sub-issue list is the source of truth for what's left.
