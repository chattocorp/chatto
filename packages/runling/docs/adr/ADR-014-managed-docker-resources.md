# ADR-014: Explicit ownership of Docker resources

**Status:** Accepted
**Date:** 2026-11-09

## Context

Workflows need disposable command environments and services. CI is one use, but
test databases and agent workspaces need the same resource lifecycle. These
operations must fit Runling's task model and cooperative cancellation.
Image builds also need streaming output, cancellation, and temporary metadata.
Those mechanics belong to the framework rather than each CI application.

## Decision

Provide Docker operations through the optional `runling/docker` module. Use the
Docker CLI and its selected context. Importing the module does not connect to a
daemon. No Docker SDK or new scheduler is required.

Image builds return immutable image IDs. Runling owns the CLI lifetime and
private image-ID files. Images and build caches remain caller-owned. Builds
and container logs share the same bounded streaming process boundary.
Applications select build directories, tags, and public build arguments.

Each operation takes the task context as its first argument. It uses that
context for cancellation and run labels. Docker context and application labels
are options on each operation. There is no public client factory or adapter
class; callers cannot omit task cancellation while configuring a client.
Containers and networks are
asynchronous disposable resources. Parents release resources after their child
tasks, as required by [ADR-009](ADR-009-task-invocations.md). A container failure
or cancellation also starts cleanup. Cleanup uses independent deadlines so a
cancelled task cannot cancel resource removal.

Runling records container activity through existing command events and streams
output through command log entries. The adapter does not record command
arguments or environment values. Environment values use private temporary files
and exact-value masking across output chunks. Applications remain responsible
for private output that the adapter cannot identify.

Resource labels identify Runling ownership and, when supplied, the workflow run.
The adapter removes only resources that it created. It does not prune resources,
restore containers from workflow snapshots, or reconcile resources after a crash.
Applications own recovery and build policy under
[ADR-010](ADR-010-workflow-state-snapshots.md).

## Consequences

- Local development can use OrbStack, Docker Desktop, or another Docker daemon
  selected through the CLI.
- Applications control images, credentials, bind mounts, networks, and resource
  limits. The adapter is not a security policy for untrusted repositories.
- Docker must be installed separately. Daemon failures can prevent cleanup;
  labels let operators find resources that remain.
- Container creation and start complete before cancellation is observed, with
  a separate control deadline. This reduces races with daemon-side creation.
  A lost response or forced process exit can still leave a labelled resource.
- Docker remains independent of Chatto, Authling, and any GitHub integration.
