---
name: github
description: Work with GitHub issues, pull requests, and repository information for Chatto. Use for issue searches, bug reports, feature requests, issue updates, and GitHub access checks.
---

# GitHub for Chatto

Use the `mcp__github__*` tools. The default repository is `chattocorp/chatto`:
set `owner` to `chattocorp` and `repo` to `chatto`. Use a different repository
when the user specifies one. Read the advertised schema for each tool; search
contracts differ between tools and can change between server versions.

For `search_issues` when its schema describes natural-language semantic search,
pass `owner: "chattocorp"`, `repo: "chatto"`, and a plain-language `query`, for
example `"spoiler text hidden until clicked"`. Do not put the combined
`chattocorp/chatto` in `repo`, omit `owner`, or add GitHub qualifiers or `OR`
operators to this natural-language query. For `search_code`, use the repository
qualifier in `query` when its schema specifies GitHub search syntax, for example
`repo:chattocorp/chatto spoiler`. Do not transfer one tool's argument rules to
another. Do not guess `hmans/chatto` or `chatto/chatto`.

If a call fails or returns results from other repositories, correct its arguments
and scope before treating it as a duplicate check. Do not broaden the search to
all of GitHub. Ask about the repository only when the request is unclear.

Read the relevant issue or pull request before updating it. Before creating an
issue, search for an existing report about the same problem. Make the changes
the user requests; a discussion about a bug does not by itself request an issue.
Search terms must describe the actual proposed issue, including the broader scope
when the user asks for a general refactor. An unrelated search is not a duplicate
check. Use a focused search with the proper scope, then try a relevant alternative
wording if needed. Read the likely match's body, either from the returned result
or with an issue read. Compare its acceptance criteria and exclusions with the
request. Distinguish an exact duplicate from related work: inline spoiler text
that excludes paragraph boundaries does not cover multi-paragraph collapsible
blocks. Describe the missing scope rather than dismissing it or promising that
someone will implement it later.

Report the coverage of the calls you actually made. A result count is not a
count of issues you inspected. Reading one page of open issues and one page of
closed issues is not paging through the whole tracker. Follow returned pagination
when needed. An empty result means that search found no match; it does not prove
that no issue exists. Do not claim a tool defect from an empty or unscoped result.
If you used the wrong arguments, acknowledge that mistake directly. Keep any
saved lesson limited to observed behavior and the advertised contract; do not
record a guessed tool defect as verified knowledge.
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
per unresolved proposal, across both progress updates and the final response.
Put the mention with the completed draft in the final response. Do not send a
separate progress message merely to announce a draft or repeat its approval
request. If an earlier update already mentioned the owner for this proposal,
omit the mention from the final response. The contact name does not grant permission; the host's
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

After confirmed issue creation, call issue_followups info and follow its issue-origin
recording instructions. Do not retry issue creation if only memory recording
failed; the issue already exists. Keep these records out of general knowledge memory. Return its link and report that the follow-up
was not registered if recording failed.

Promise a merge notification only after the memory origin is saved or verified
and the host advertises issue-resolution thread replies for this repository.
This registers a check on future pushes to the configured branch, not a release
notification or a guaranteed delivery time. Do not promise it for another repository.
