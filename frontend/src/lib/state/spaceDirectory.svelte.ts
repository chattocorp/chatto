import { SvelteMap } from 'svelte/reactivity';
import type { Client } from '@urql/svelte';
import { graphql, useFragment } from '$lib/gql';
import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
import { SpaceCardSpaceFragmentDoc, type SpaceCardSpaceFragment } from '$lib/gql/graphql';
import type { RegisteredInstance } from '$lib/state/instance/registry.svelte';

export type InstanceSpaceData = {
  instanceId: string;
  instanceName: string;
  loading: boolean;
  canBrowse: boolean | null;
  spaces: SpaceCardSpaceFragment[];
  error: string | null;
};

export type JoinSpaceResult = { ok: true } | { ok: false; error: Error };

const LoadInstanceSpacesQuery = graphql(`
  query LoadInstanceSpaces {
    spaces {
      ...SpaceCardSpace
    }
    viewer {
      canListSpaces
    }
  }
`);

const JoinSpaceFromDirectory = graphql(`
  mutation JoinSpaceFromDirectory($input: JoinSpaceInput!) {
    joinSpace(input: $input)
  }
`);

/**
 * Minimal interface the store needs to look up per-instance urql clients.
 * Defaults to {@link graphqlClientManager}; tests pass a stub.
 */
export type ClientManager = { getClient: (instanceId: string) => { client: Client } };

/**
 * Reactive state for the cross-instance Browse Spaces directory.
 *
 * Owns the per-instance load result map (`SvelteMap<instanceId,
 * InstanceSpaceData>`) plus the optimistic `joiningKey` for an in-flight
 * join. Components issue {@link loadAll} from a `$effect` that tracks the
 * live set of authenticated instances; the store keeps already-loaded
 * instances stable across re-runs and only refetches when the list changes.
 *
 * Cross-instance shape (cf. `DMConversationsStore`): the store imports the
 * `graphqlClientManager` singleton by default rather than taking a single
 * `Client` in its constructor — but the manager interface is injectable for
 * tests.
 */
export class SpaceDirectoryStore {
  instanceData = new SvelteMap<string, InstanceSpaceData>();
  joiningKey = $state<string | null>(null);

  constructor(private readonly clientManager: ClientManager = graphqlClientManager) {}

  /**
   * Fetch spaces for the supplied set of authenticated instances.
   *
   * - Drops entries for instances that have disappeared since the last call.
   * - Sets `loading: true` for the rest, then dispatches per-instance
   *   queries in parallel. Each result lands independently — a slow instance
   *   doesn't hold up the rest.
   *
   * Typically invoked from a `$effect` whose dependency is a `$derived`
   * "authenticated instances" list, so the load re-fires when sign-in state
   * or the instance list changes.
   */
  async loadAll(authenticatedInstances: readonly RegisteredInstance[]): Promise<void> {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local computation, not reactive state
    const currentIds = new Set(authenticatedInstances.map((i) => i.id));
    for (const id of this.instanceData.keys()) {
      if (!currentIds.has(id)) this.instanceData.delete(id);
    }

    for (const inst of authenticatedInstances) {
      this.instanceData.set(inst.id, {
        instanceId: inst.id,
        instanceName: inst.name,
        loading: true,
        canBrowse: null,
        spaces: [],
        error: null
      });
    }

    await Promise.all(
      authenticatedInstances.map(async (inst) => {
        try {
          const client = this.clientManager.getClient(inst.id).client;
          const result = await client.query(LoadInstanceSpacesQuery, {}).toPromise();

          const current = this.instanceData.get(inst.id);
          // Instance disappeared (was removed mid-load) — drop the result.
          if (!current) return;

          if (result.error) {
            this.instanceData.set(inst.id, {
              ...current,
              error: result.error.message,
              loading: false
            });
            return;
          }

          if (result.data) {
            const canList = result.data.viewer?.canListSpaces ?? false;
            const spaces = canList
              ? (result.data.spaces?.map((s) => useFragment(SpaceCardSpaceFragmentDoc, s)) ?? [])
              : [];
            this.instanceData.set(inst.id, {
              ...current,
              canBrowse: canList,
              spaces,
              loading: false
            });
            return;
          }

          this.instanceData.set(inst.id, { ...current, loading: false });
        } catch (err) {
          const current = this.instanceData.get(inst.id);
          if (!current) return;
          this.instanceData.set(inst.id, {
            ...current,
            error: err instanceof Error ? err.message : 'Failed to connect',
            loading: false
          });
        }
      })
    );
  }

  /**
   * Join a space on a specific instance. Sets `joiningKey` to
   * `${instanceId}:${spaceId}` for the duration of the request so the
   * corresponding card can render its in-flight state.
   */
  async joinSpace(instanceId: string, spaceId: string): Promise<JoinSpaceResult> {
    this.joiningKey = `${instanceId}:${spaceId}`;
    try {
      const client = this.clientManager.getClient(instanceId).client;
      const result = await client
        .mutation(JoinSpaceFromDirectory, { input: { spaceId } })
        .toPromise();

      if (result.error) {
        return { ok: false, error: new Error(result.error.message) };
      }
      return { ok: true };
    } finally {
      this.joiningKey = null;
    }
  }
}
