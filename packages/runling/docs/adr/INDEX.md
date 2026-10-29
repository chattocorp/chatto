# Runling Architecture Decision Records

These records belong to Runling and use independent numbering. See the
[documentation index](../README.md) for scope and API guides.

| Number  | Decision                                                                                       | Status                         | Date       |
| ------- | ---------------------------------------------------------------------------------------------- | ------------------------------ | ---------- |
| ADR-001 | [Process-local TypeScript workflows](ADR-001-process-local-workflows.md)                       | Amended by ADR-010             | 2026-09-22 |
| ADR-002 | [Explicit agent connections and child ownership](ADR-002-agent-ownership.md)                   | Amended by ADR-009             | 2026-09-22 |
| ADR-003 | [Separate task state, output, and notifications](ADR-003-task-context.md)                      | Superseded by ADR-009          | 2026-09-22 |
| ADR-004 | [Transport-neutral event sources](ADR-004-event-sources.md)                                    | Amended by ADR-009             | 2026-09-22 |
| ADR-005 | [Block tools after untrusted context](ADR-005-untrusted-context.md)                            | Accepted                       | 2026-09-27 |
| ADR-006 | [Tasks communicate only with their parent and children](ADR-006-parent-child-communication.md) | Amended by ADR-009             | 2026-09-28 |
| ADR-007 | [Classify authorization in a separate model call](ADR-007-authorization-classifier.md)         | Withdrawn                      | 2026-09-30 |
| ADR-008 | [Parents own approval of delegated actions](ADR-008-parent-owned-approvals.md)                 | Withdrawn                      | 2026-10-03 |
| ADR-009 | [Typed task invocations are the execution model](ADR-009-task-invocations.md)                  | Amended by ADR-010 and ADR-012 | 2026-10-14 |
| ADR-010 | [Periodic workflow state snapshots](ADR-010-workflow-state-snapshots.md)                       | Amended by ADR-012             | 2026-10-18 |
| ADR-011 | [In-process hosting and keyed batching](ADR-011-hosting-and-batching.md)                       | Accepted                       | 2026-10-26 |
| ADR-012 | [Router-controlled workflow reactivation](ADR-012-router-controlled-reactivation.md)           | Accepted                       | 2026-10-27 |
| ADR-013 | [Source exports in the workspace](ADR-013-source-workspace-exports.md)                         | Accepted                       | 2026-10-27 |
