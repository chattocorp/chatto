---
name: chatto-pr-checklist
description: Check a Chatto pull request before review or merge. Use when opening a PR or when asked to check one.
---

# Chatto PR Checklist

Review the complete branch diff against the target branch.

- Check tests for the changed behavior. Add missing coverage within the task.
- Check the documentation listed in root `AGENTS.md`. Update affected records
  and public docs within the task.
- For public API or protocol changes, use
  [chatto-api-compatibility](../chatto-api-compatibility/SKILL.md).
- Follow root `AGENTS.md` for commit titles, PR status, and issue links.

For a review-only request, report findings without edits. Report only items
that need action. This checklist does not authorize changes to agent
instructions, branch names, or remote repositories.

## PR Description

Explain the problem, resulting behavior, and important design decisions.
List the checks run, their results, and any checks still needed. State relevant
compatibility, migration, security, or deployment effects.

Use enough detail for the change. After creating or updating a PR, read its
stored description and check that it matches the complete diff.
