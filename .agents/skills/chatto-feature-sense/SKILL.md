---
name: chatto-feature-sense
description: Decide who a new or changed Chatto feature affects, whether it starts as an experiment, and how to ship and communicate it. Use when you plan a feature, before you build a user-visible change, and before you open its PR.
---

# Chatto Feature Sense

Use this skill to think about a feature as a product change, not only as code.
Answer the questions below and give a short **feature brief** (see
[Output](#output)). Do this before implementation when possible. Check the
brief again before you open the PR, because scope often changes.

This skill gives recommendations. It does not authorize a release, a public
announcement, or a change to server compatibility thresholds.

## 1. Who does it affect?

Identify each affected audience. For each, state what changes for them and
whether they must do something.

| Audience | Typical surfaces |
| --- | --- |
| **Member** | Room View, composer, threads, notifications, App Preferences |
| **Guest** | Shared rooms and messages without an account |
| **Moderator** | Room removal, suspensions, slow mode, message management |
| **Administrator** | Server Configuration, roles, invitations, bot accounts |
| **Operator** | Configuration files, environment, deployment, upgrade, backup, resources, monitoring |
| **Integration author** | Public API, realtime events, `@chatto/client`, bots, webhooks, MCP |

Also check these dimensions:

- **Platforms.** Browser, desktop, touch devices. Different behavior per
  platform needs a reason.
- **Mixed versions.** A new client connects to old servers, and old clients
  connect to new servers. Decide what each combination shows. Use capability
  discovery. Do not raise the connection floor
  ([FDR-031](../../../docs/fdr/FDR-031-client-server-compatibility-discovery.md),
  root `AGENTS.md`).
- **Multiple servers.** App Preferences apply to every server in the app.
  Server Configuration applies to one server only.
- **Privacy.** Name each external system that the feature contacts and the
  data it reveals, including IP addresses. Tell the user about necessary
  connections.

## 2. What kind of change is it?

Classify the change. The class controls most later decisions.

- **New capability.** Nothing existing changes. Low risk for habits.
- **Changed behavior.** Existing users must learn something new or lose a
  habit. Highest communication need.
- **Removed behavior.** Always state the replacement or the reason.
- **Fix.** Name the problem that the reader experienced.
- **Invisible.** Refactor, performance, storage. No audience communication.
  Record it in ADRs, FDRs, or architecture docs as applicable.

Then answer:

- **Default.** Is it on for everyone, opt-in, or opt-out? Every opt-in needs
  a path to opt out again, and every opt-out needs a path back in. Never
  enable a stored opt-out again silently.
- **Reversibility.** Does it write durable events, persisted protobufs, or a
  public API contract? If yes, it is difficult to remove. An experiment does
  not make a persisted contract reversible. Get the contract right first,
  in an FDR or ADR.
- **Permissions.** Which permission controls it? Does it change what an
  existing role can do?

## 3. Should it start as an experiment?

Select one rollout mode.

| Mode | Use when |
| --- | --- |
| **Ship directly** | Fixes, clear improvements, new capabilities that change no habits, and changes with no reasonable alternative design. This is the default. |
| **Client experiment** ([chatto-experiments](../chatto-experiments/SKILL.md), [FDR-059](../../../docs/fdr/FDR-059-client-experimental-features.md)) | A per-user presentation or interaction change in the bundled frontend. The design is uncertain, or it changes a habit for many members. Both branches can exist without persisted data differences. |
| **Operator opt-in** (configuration, disabled by default) | The feature adds a new network surface, contacts a third party, has a significant resource cost, or has an unstable public contract. Example: the MCP server. |
| **Administrator setting** | A server community must be able to choose the behavior. The choice is permanent, not a trial. Use Server Configuration and permissions. |

Rules for every experiment:

- State the question that the experiment answers and the signal that ends it.
- State the exit: adopt (make it the only behavior) or retire (remove it).
  Do not keep experiments indefinitely.
- Do not use an experiment to avoid a design decision about persisted data
  or the public API.

## 4. How do we ship it?

- **Release note.** Add a Sweet note for each affected release unit when the
  change is user-visible (`mise x -- sweet change <package-path>`). Write for
  the reader identified in step 1. Follow
  [Writing change notes](../../../CONTRIBUTING.md#writing-change-notes).
  Select the type: `fix`, `feature`, or `breaking`.
- **Upgrade actions.** If an operator or integration author must do
  something, state it in the note and in the PR.
- **Compatibility evidence.** For API or protocol changes, use
  [chatto-api-compatibility](../chatto-api-compatibility/SKILL.md).

## 5. How do we tell people?

Match each audience to a channel. Use only the channels that the audience
needs.

| Audience | Channel |
| --- | --- |
| Member, moderator, administrator | Sweet note. Release announcement if noteworthy. In-app discovery, such as an empty state, hint, or label, when the feature is hard to find. |
| Operator | Sweet note with upgrade actions. Docs website when installation, configuration, security, monitoring, upgrades, troubleshooting, or recovery changes. |
| Integration author | Sweet note with migration steps. Generated API reference. Docs website when a public integration contract changes. |
| Experiment participants | The Experiments label and description. No release announcement until adoption. |

Always update the relevant FDR with the final behavior and rationale. Mark
the feature as a highlight for the
[release announcement](../chatto-release-announcement/SKILL.md) only when a
reader would want to try it or must act on it.

## Output

Give the brief in this format. Keep each line short. Omit sections that do
not apply.

```markdown
## Feature brief: <name>

- **Audiences:** <audience> — <what changes>; <action required or none>
- **Change class:** <new | changed | removed | fix | invisible>
- **Default and reversibility:** <on/opt-in/opt-out>; <durable data or API: yes/no>
- **Mixed versions:** <behavior with old server / old client>
- **Privacy:** <external connections or none>
- **Rollout:** <ship directly | client experiment | operator opt-in | administrator setting> — <reason>
- **Experiment exit:** <question, signal, adopt/retire> (experiments only)
- **Release note:** <release unit, type, one-line draft>
- **Communication:** <channel per audience>
- **Records:** <FDR/ADR/docs to update>
- **Open questions:** <decisions for the maintainer>
```

Ask the maintainer about open questions that change the rollout mode or a
public contract. Do not decide them yourself.
