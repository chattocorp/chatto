import { createContext } from 'svelte';

/**
 * Svelte context for the active space ID — the deployment's primary space
 * (future Server). Set by the [[instanceId]] layout from
 * `instanceStore.instance.primarySpaceId`, which the GetInstanceInfo query
 * loads at instance bootstrap.
 *
 * Value is a getter so consumers see the latest primarySpaceId once the
 * query resolves. Empty string while loading or on fresh installs with no
 * user-facing space yet.
 *
 * Migration bridge for ADR-027 / #330: collapses the URL's `[spaceId]`
 * param. Will be removed once Instance + Space have merged into Server.
 */
export const [getActiveSpace, setActiveSpace] = createContext<() => string>();
