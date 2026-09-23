# ADR-004: Transport-neutral event sources

**Status:** Accepted
**Date:** 2026-09-22

## Context

Some workflow hosts receive webhooks. Others consume sockets or subscriptions
and need to start before an HTTP request arrives. Runling must not depend on a
specific chat protocol or duplicate delivery routing for each transport.

## Decision

Web configurations can declare named source functions independently of webhooks.
Each source receives an abort signal, a retained state map, and a dispatch
function. Sources and HTTP routes share the same run store and routing logic.
Each delivery gets a fresh routing context. Dispatch waits for run registration,
not workflow completion.

Start sources eagerly in development and packaged serving. For valid configuration
reloads, stop and await old sources before starting replacements. Preserve state
for names present in both configurations. Invalid configurations leave existing
sources running. Source removal or renaming discards the old source state.

Adapters own authentication, transport retries, checkpoints, deduplication, and
application routing. Runling records source failures safely and leaves a failed
source stopped until reload. Sources must release subscriptions, sockets, and
timers when aborted.

## Consequences

A source can use a transport without adding that transport to Runling. Reload
handoff prevents overlapping source instances, but depends on cooperative cleanup.
Retained state needs adapter-owned identity scoping and compatibility across
reloads. Process-local state provides no restart recovery or delivery guarantee.
