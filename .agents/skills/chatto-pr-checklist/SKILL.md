---
name: "chatto-pr-checklist"
description: "Pre-merge PR checklist for the Chatto codebase. Contains instructions for tasks that must run before a PR is merged. This skill should automatically run when a PR is opened."
---

# Chatto PR Checklist

- Familiarize yourself with the changes in this branch/PR.
- If you're working in a branch, make sure the branch is named something descriptive of the change.
- Are there any test gaps around the new/changed functionality? If so, please fill them.
- Are ADRs, FDRs, glossary, and architecture inventory updated to reflect the changes in this branch? If not, please update them.
- Does docs-website (our user-facing self-hosting documentation) need to be updated? If so, please update it.
- Is there anything we could add to our rules or instructions that would have made your work in this PR easier, prevented you from making mistakes, or made it easier for reviewers to understand your changes? If so, please add it to our rules or instructions.

## Breaking Changes Checklist

- If this PR contains changes to our protocol buffers, please notify the user.
- If this PR contains changes to our GraphQL API, please notify the user.
- If this PR contains any other changes that you feel might be a breaking change, please notify the user.
