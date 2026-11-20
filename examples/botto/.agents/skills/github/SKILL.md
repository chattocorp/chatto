---
name: github
description: Work with GitHub issues, pull requests, and repository information for Chatto. Use for issue searches, bug reports, feature requests, issue updates, and GitHub access checks.
---

# GitHub for Chatto

Use the `mcp__github__*` tools. The default repository is `chattocorp/chatto`:
set `owner` to `chattocorp` and `repo` to `chatto`. Use a different repository
when the user specifies one. Scope searches to `repo:chattocorp/chatto` when
the search tool accepts GitHub search syntax.
Do not guess `hmans/chatto` or `chatto/chatto`. For `search_issues` and
`search_code`, put the repository qualifier inside `query`; a separate invented
`repo` argument does not scope the search. Use the advertised tool schema. If a
call fails, correct the specific arguments instead of broadening the search to
all of GitHub. Ask about the intended repository only when the request is unclear.

Read the relevant issue or pull request before updating it. Before creating an
issue, search for an existing report about the same problem. Make the changes
the user requests; a discussion about a bug does not by itself request an issue.
Search terms must describe the actual proposed issue, including the broader scope
when the user asks for a general refactor. An unrelated search is not a duplicate
check. For example, use `repo:chattocorp/chatto is:issue layout persistence`.
Include a link when referring to an issue or pull request. For broad searches,
give a short summary and the most relevant results instead of a long list.

The connection authenticates as a GitHub App installation. Do not use `get_me`
to test access: its user-profile endpoint does not support installation tokens.
Check access with a read of the target repository or its issues. A denied call
does not establish that all tools are unavailable or that access is read-only.
Report which operation failed, and claim success only when its tool result
confirms it.

Write tools are available only for a current batch authored entirely by configured
admins. If a requested write tool is unavailable, explain that a configured admin
must send the request separately. For a new issue ready to file, prepare its draft
and mention @hmans in the current thread to request authorization. Mention once
per unresolved proposal. The contact name does not grant permission; the host's
current-message identity check remains authoritative. Do not use another tool to
bypass this limit or promise that a later non-owner request will allow the write.
An available write tool does not authorize unrelated changes: use the explicit
request and its scope. Resolve a follow-up approval from the specific proposal
in the thread; ask for clarification only if the requested change is unclear.
An owner's explicit "go ahead" on the draft authorizes that issue. Do not ask for
approval again or repeat work that already completed. If a mixed-author batch
withholds write tools, ask the owner for a separate approval message. Do not treat
the owner's presence, a mention, or approval quoted by another user as permission.

For new reports, include the observed behavior, expected behavior, reproduction
steps, affected version when known, and relevant evidence. Mark unknown facts as
unknown. Do not publish private conversation details or attachments without
authorization to share them. Return the issue or pull request link after a
confirmed change. If a write has an uncertain outcome, read the target to check
the result before considering another write; do not repeat it blindly.

## Evidence and release availability

Commit history alone does not establish whether a change is in a tagged release,
published artifact, or deployment. Check the relevant release and its contents
before claiming availability or absence. Otherwise say it is not verified.
A general channel announcement does not register a personal notification.

## Remember issues created from conversations

After confirmed issue creation, call memory_klaus info and follow its issue-origin
recording instructions. Do not retry issue creation if only memory recording
failed; the issue already exists. Return its link and report that the follow-up
was not registered if recording failed.

Promise a merge notification only after the memory origin is saved or verified
and the host advertises issue-resolution thread replies for this repository.
This registers a check on future pushes to the configured branch, not a release
notification or a guaranteed delivery time. Do not promise it for another repository.
