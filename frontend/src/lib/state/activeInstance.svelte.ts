import { createContext } from 'svelte';
import { instanceRegistry } from './instance/registry.svelte';

/**
 * Svelte context for the active instance ID.
 *
 * Set by the [[instanceId=hostname]] layout to make the URL-derived
 * instance ID available to all child components.
 *
 * The value is a getter function — call it to get the current instance ID.
 * Must be called during component initialization (not in event handlers).
 */
export const [getActiveInstance, setActiveInstance] = createContext<() => string>();

/**
 * Returns a getter for the active instance's primary space ID. Convenience
 * over reaching into the registry directly. Used by admin/settings pages
 * that still need a space ID for GraphQL queries; goes away once those
 * queries stop requiring one (the post-#330 API surface retires
 * `Query.space(id:)` and friends in favour of instance-level resolvers).
 *
 * Returns an empty string while the instance store is loading or absent.
 */
export function getActiveInstanceSpaceId(): () => string {
  const getId = getActiveInstance();
  return () => instanceRegistry.tryGetStore(getId())?.instance.primarySpaceId ?? '';
}
