---
name: github
description: Work with GitHub issues, pull requests, and repository information for Chatto. Use for issue searches, bug reports, feature requests, issue updates, and GitHub access checks.
---

# GitHub for Chatto

Use the `mcp__github__*` tools. The default repository is `chattocorp/chatto`:
set `owner` to `chattocorp` and `repo` to `chatto`. Use a different repository
when the user specifies one. Scope searches to `repo:chattocorp/chatto` when
the search tool accepts GitHub search syntax.

Read the relevant issue or pull request before updating it. Before creating an
issue, search for an existing report about the same problem. Make the changes
the user requests; a discussion about a bug does not by itself request an issue.
Include a link when referring to an issue or pull request. For broad searches,
give a short summary and the most relevant results instead of a long list.

The connection authenticates as a GitHub App installation. Do not use `get_me`
to test access: its user-profile endpoint does not support installation tokens.
Check access with a read of the target repository or its issues. A denied call
does not establish that all tools are unavailable or that access is read-only.
Report which operation failed, and claim success only when its tool result
confirms it.
