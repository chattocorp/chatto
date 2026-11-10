# Git working trees

Import Git helpers from `runling/git`. Git must be installed on the host.

## Select current source files

```ts
import { workingTreeManifest } from 'runling/git';

await using source = await workingTreeManifest(ctx, '/absolute/path/to/repository');
// source.path is a NUL-separated file list, suitable for tar --null -T.
// Keep this scope open until every consumer has stopped using the file.
```

The manifest includes tracked and unignored files, including local edits and
new files. It excludes deleted files, Git metadata, and submodule directories.
Tracked files remain included even if an ignore rule matches them. Select
submodule worktrees separately when their source is needed.

Runling creates the manifest in private temporary storage. Disposal removes its
file and directory. Selection observes `ctx.signal`; a cancelled selection
does not return a resource. The helper does not write into the repository.

The manifest selects paths, not file contents. Consumers read current content
when they use those paths. Avoid concurrent edits when a run must use one
consistent source tree. A Docker bind mount still exposes its whole source
directory; the manifest controls only the consumer's file selection.
