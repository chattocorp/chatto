# Workflow state

`runling serve` owns one state snapshot for the instance. A root task receives its
own mutable object as `ctx.state`. Change it with ordinary JavaScript:

```ts
import { task, Type } from 'runling';
import { defineWebConfig, startWorkflow } from 'runling/web';

const counter = task(
  { name: 'Counter', input: Type.Null(), result: Type.Number(), steering: Type.Number() },
  async (ctx) => {
    ctx.state.count ??= 0;
    for await (const amount of ctx.steering) {
      if (typeof ctx.state.count !== 'number') throw new Error('Invalid saved count');
      ctx.state.count += amount;
    }
    return Number(ctx.state.count);
  }
);

export default defineWebConfig({
  workflows: { counter },
  webhooks: { counter: startWorkflow(counter) }
});
```

The configuration key `counter` identifies the root after restart. Use the same
task object in the registry and the router. Keep the key stable when you change a
task's display name. Factory-created tasks work too: create the task once, then
register and route to that object.

On restart, the server calls the root again with its original input and saved
state. The example preserves the last saved count and waits for new messages.
`ctx.run?.resumed` is true on server recovery, including when the saved state is
still empty. Use it when the original input must not start an external action again.
Runling does not restore the prior inbox. The root must decide how to reconnect
resources, migrate its state, and reconcile actions whose outcome is uncertain.

## Run titles

Recorded workflows can set a descriptive title with
`ctx.run?.setTitle('Compare JSON and SQLite')` and read it through `ctx.run?.title`.
The setter normalizes whitespace and accepts 1–120 Unicode characters. It rejects
empty values, other control characters, and updates after execution ends. Repeating
the same title does not emit another event. Calls record updates through the host's
normal journal path; they do not wait for disk persistence.

The console displays the title instead of the workflow type as the main heading.
The type and stable run reference remain unchanged. Titles survive restart and
reactivation. They render as plain text and are visible in the run list, so exclude
secrets and personal details. Older runs need no migration and fall back to their
workflow type. Applications can wrap this operation in an agent tool; Runling has
no automatic naming policy. See [FDR-013](fdr/FDR-013-run-titles.md).

## Reactivate a finished workflow

A root return ends its execution. To reuse its identity and state, a router calls
`ctx.start(root, { input: nextInput, runId })`. Register `root` in `workflows` and
use the same source kind and route name that created the run. Reactivation rejects
while another execution is active. Queue inputs in the application when needed.

The new execution uses fresh input and retained state. `ctx.run.resumed` is false;
that flag identifies recovery after server interruption. Completed, failed, and
cancelled executions are not restarted automatically. Their handles do not
control later executions. Earlier inputs, results, and events remain in the
journal. The console retains logs and timeline activity across executions, with
total usage. Input, result, and status describe the latest execution.

Sources can find retained runs through `ctx.runs({ includeFinished: true })`.
Each entry supplies the latest input for application routing. The default
`ctx.runs()` still returns active runs only. A finished entry's `finished` promise
is settled, `steer` rejects, and cancellation and closing steering do nothing.

No snapshot migration is required. To adopt this lifecycle, let the root return
after its work and have the router select the retained ID for the next input.
Interrupted roots still follow the recovery rules above. If retention has removed
the run, the router must start a new run without `runId`.

## Saving and shutdown

Set `RUNLING_DATA_DIRECTORY` before starting the host to select a separate
storage directory for an instance. Run journals and snapshots go in its `runs/`
subdirectory; server diagnostics go in `logs/`. An absolute path is recommended.
Relative paths resolve from the server configuration directory, or from the
working directory for `runling run`. Without this setting, existing `.runling/`
and legacy journal-directory behavior is unchanged. Keep one owning process
per storage directory. Restart the host after changing this setting.

The server writes `.runling/runs/state.json` every 60 seconds, when an execution
ends, and after task cleanup at graceful shutdown. Projects that use the legacy journal directory
keep their snapshot there. Each write captures the complete object, writes a
private temporary file, then renames it over the previous snapshot. Writes are
ordered. There are no proxies or explicit save calls in task code.

Store plain JSON values only. Functions, sockets, promises, dates, maps, cycles,
and `undefined` values are not state. Use strings for dates and IDs for external
resources. Invalid state fails the complete snapshot rather than silently losing
properties. State can contain private application data; keep the journal directory
private and out of version control.

Abrupt exit can lose recent mutations and newly started workflows. A failed save
can make that window longer. A snapshot is not a transaction with external
services and does not guarantee durability after power loss. Check external
outcomes before repeating actions. Completed, failed, and cancelled journals take
precedence over an older snapshot that says the workflow is still open.

Shutdown leaves active workflows open for recovery. It waits for cooperative
cleanup before the final snapshot; tasks must honor cancellation. Only one server
may own the snapshot file.

## Retention

At server startup, and once per day through the
persistence timer, Runling purges runs closed more than 30 days ago. Closure means
completion, failure, or explicit cancellation. The latest journal's `finishedAt`
sets the age. Running and interrupted workflows are preserved regardless of age.
Applications still decide when their root should finish.

Cleanup deletes the state entry, journal, and run-owned file directory, including
those from one-shot CLI runs in the same directory. A missing or unreadable journal cannot establish
closure, so its state is preserved. There is no separate scheduler or model call.

Recorded runs receive `ctx.run.filesDirectory`, located at `.runling/runs/<run-id>/`
(or beside the journal in the legacy directory). Runling owns its retention.
Use `agentSessionFile(ctx, key)` for stable, private transcript paths inside it.
Applications select a session key; Runling creates the directory and removes it
with the expired run. Files at arbitrary external `sessionFile` paths remain
caller-owned. No application cleanup callback is needed.

Filesystem failures are logged and do not stop startup or ordinary saves.
A completed purge makes the run unavailable in the console.

The reduced state snapshot is saved before run files and journals are deleted. If saving fails,
the state entries are restored in memory. If the process stops after the snapshot
save, remaining journals permit cleanup to retry at the next startup. The journal
is deleted last. Shutdown
waits for a cleanup already in progress before its final save.

## Child tasks

Children do not inherit the root's full state. Pass a selected sub-object as
ordinary task input when a child needs to update saved state:

```ts
ctx.state.implementation ??= { phase: 'new' };
await implementation(ctx, ctx.state.implementation);
```

The child's input schema must accept that object. A transforming schema can copy
or replace it, so use a validator that preserves the reference when sharing state.
Children's own `ctx.state` objects are empty and unsaved. Root recovery chooses
which children to start; saved sub-objects do not cause automatic child recovery.

## Sources and other hosts

Sources start after registered roots recover. `ctx.runs()` returns live runs owned
by that source, including their original `input`, registry key in `workflow` (or
display name for unregistered roots), and the usual run handle methods. Use this
to rebuild a thread or session routing table. Source `ctx.state` remains an
in-memory map; it is separate from task `ctx.state`.

Unregistered server roots also have snapshot state, but cannot restart
automatically. A missing registry key preserves its saved workflow and leaves its
journal interrupted. Register only roots that can handle re-entry; the registry
does not make an existing task safe to repeat.

`runling run` remains a one-shot CLI execution without automatic recovery. Library
hosts can pass `runWorkflow(root, { input, state })`; the supplied object is used
directly, and that host owns loading and saving it. Agents still need their own
session files. This state API does not save model sessions automatically.
