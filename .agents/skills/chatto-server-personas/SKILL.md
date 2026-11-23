---
name: chatto-server-personas
description: Check Chatto features, configuration, and operator documentation against typical self-hosted server personas, such as a single host, a Docker Compose stack, Chatto Cloud, a high-availability cluster, an SSO-only company server, or an AT Protocol-only community. Use to audit setup, initialization, configuration, and administration gaps, or to check a new, changed, or removed feature for each type of server.
---

# Chatto Server Personas

A **server persona** is a typical type of self-hosted Chatto server and the
operator who runs it. [`personas.md`](personas.md) is the catalog. Each persona
states its goal, its setup, and the requirements that Chatto must satisfy for
it.

Use the personas to find gaps that a single development setup does not show.
For example, the development stack always has password login, a local email
server, and a terminal on the host. Many real servers have none of these.

## When To Use This Skill

- **Feature check.** A change adds, changes, or removes an operator-visible
  feature: configuration, setup, login, roles, storage, backups, upgrades,
  external connections, or the admin UI. Check the change against each
  persona that it can affect.
- **Periodic audit.** The user asks for a review of self-hosting readiness.
  Audit all personas, or the personas that the user selects.
- **Planning.** The user asks who a deployment or identity feature is for.
  Use the personas to name the affected servers.

## Procedure

1. Read [`personas.md`](personas.md). Select the personas in scope. For a
   feature check, select each persona where the change touches one of its
   requirements. Say which personas you skipped and why. Do not report needs
   that only the servers in its **Out Of Scope** section have.
2. For each selected persona, examine each **lifecycle stage**:

   | Stage | Question |
   | --- | --- |
   | Install | Can the operator get Chatto and its dependencies running in this environment? |
   | Initialize | Can the operator create the first owner and complete setup without a prohibited dependency? |
   | Configure | Can the operator set each required option with the persona's configuration method? |
   | Administer | Can administrators do daily work, such as onboarding, offboarding, roles, and moderation? |
   | Upgrade | Can the operator upgrade safely, with the persona's availability needs? |
   | Back up and restore | Can the operator back up all data and restore it in this environment? |
   | Recover | Can the operator recover access when the primary login method or a dependency fails? |

3. For each stage, find the evidence. Read the public documentation in
   `apps/docs-website/`, the examples in `examples/`, the configuration in
   `cli/internal/config/`, the CLI commands in `cli/cmd/`, and the relevant
   FDRs. Do not describe behavior from memory.
4. Rate each stage:
   - ✅ **Supported and documented.** The persona can do it with the public
     documentation.
   - ⚠️ **Possible but undocumented or awkward.** It works, but the operator
     must guess, use a workaround, or read the source code.
   - ❌ **Not possible.** A requirement of the persona has no supported path.
5. Mark each finding as **verified** (you read the code or documentation that
   proves it) or **inferred** (you did not confirm it). Do not report an
   inferred gap as fact.
6. Report the results (see [Output](#output)).

Do not deploy a persona's environment unless the user asks. Agents run on the
user's workstation. Do not start heavy or parallel workloads to test a persona.

## Output

Give a short result for each persona:

```markdown
### <Persona name>

| Stage | Rating | Evidence |
| --- | --- | --- |
| Initialize | ❌ | Setup wizard requires `auth.direct_login` (docs/fdr/FDR-047-first-run-setup.md) — verified |

**Gaps:** one line for each ⚠️ or ❌, with the smallest fix you can see.
```

Then give a combined list of gaps, ordered by impact. Put gaps that affect
several personas first. For a feature check, also state if the change fixes,
keeps, or adds a gap for each persona.

Do not write the gaps into this skill or into `personas.md`. Gaps change with
each release. File them as GitHub issues when the user asks.

## Maintain The Catalog

- Add a persona when a real type of server has requirements that no existing
  persona covers. Remove or merge personas that do not find different gaps.
- Write requirements as needs of the persona, not as Chatto features. For
  example, write "No person ever has a password", not "Supports
  `direct_login = false`". This keeps a requirement valid when the
  implementation changes.
- Update the `Last reviewed` date in `personas.md` after a full audit.
- Use ASD-STE100 Simplified Technical English and the terms in
  `docs/GLOSSARY.md`.
