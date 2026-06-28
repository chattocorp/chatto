# ADR-010: Svelte 5 Reactive Cache Whitelisting

**Date:** 2026-03-01

## Context

Chatto's frontend uses Svelte 5 runes (`$state`, `$derived`) for reactive state management. The room event stores receive realtime events and projected timeline refreshes, then cache renderable events for child components to consume.

A performance investigation revealed that pushing *any* item to a `$state` array triggers every `$derived` expression that reads it. Even if downstream `$derived` chains immediately filter the item out (e.g., filtering by room ID), the entire chain re-evaluates because `.filter()` always produces a new array reference. This caused CPU spikes when high-frequency events like typing indicators were added to the cache.

The initial approach used a blacklist: cache everything, then filter out unwanted event types downstream. This failed because the damage (triggering the reactive chain) was already done by the time the filter ran.

## Decision

Use a **whitelist** of cacheable event types at the cache entry point. Events that are not in the whitelist are never added to the `$state` array:

- Only events that components need for rendering (messages, joins, leaves, edits, deletions) are added to the cache
- Ephemeral signal events (typing indicators, reactions, presence updates) are handled via separate, targeted reactive state — not the main event cache
- The whitelist is checked **before** the `$state` array mutation, not after

Additionally:

- Cache expensive object construction (e.g., `Intl.DateTimeFormat` instances) at module level rather than creating them inline on every render
- When grouping or transforming cached data, ensure intermediate `$derived` steps don't create unnecessary new references

## Consequences

- **No wasted reactive cycles**: Events that won't survive downstream filters never enter the reactive graph. The `$derived` chain only re-evaluates when genuinely relevant data changes.
- **Explicit cache contract**: The whitelist makes it clear which events are cached and which are ephemeral. Adding a new event type to the cache is a conscious decision.
- **Ephemeral events need separate handling**: Typing indicators, audio levels, and similar high-frequency signals must be routed through their own state management (e.g., a dedicated `$state` map) rather than the general event cache.
- **Svelte 5 reactivity is reference-based**: This is a fundamental characteristic of the framework. Any mutation to a `$state` value triggers all readers, regardless of whether the logical content changed. All reactive state design must account for this.
- **New event types require a decision**: When adding a new event type, developers must decide whether it goes in the cache whitelist or gets separate handling. This is additional friction but prevents performance regressions.
