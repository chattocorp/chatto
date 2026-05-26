---
name: "chatto-release-announcement"
description: "Generate user-facing release notes by comparing two git tags"
---

Generate user-facing release notes for the latest release!

The file should be stored at `.docs/release-<version>.md` where `<version>` is the new git tag (eg. v0.0.54).

- Find the newest git tag (eg. v0.0.54) and the previous git tag (eg. v0.0.53)
- Inspect all commits between these two tags
- Use this information to write a release announcements.
- The release announcement should be formatted using Markdown (I will manually post it on Chatto after some editing.)
- Remember that it's user-facing, so focus on user-facing changes, improvements, and bug fixes. (No need to talk about internal refactoring unless it has a direct user impact.)
- It should be structured as a single bullet list of changes: new and changed features first, followed by fixes.
- Fixes should always start with the word "Fixed", eg. "Fixed an issue where..."
- Keep it concise and focused. No emojis.
