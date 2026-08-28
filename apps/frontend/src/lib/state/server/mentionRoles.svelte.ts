import type { RoleAPI } from '$lib/api-client/roles';
import type { MentionRole } from '$lib/state/room';

type MentionRoleAPI = Pick<RoleAPI, 'listRoles'>;

export type MentionRolesStatus = 'idle' | 'loading' | 'ready' | 'failed';

/** Shared public role catalogue used by message rendering and composers. */
export class MentionRolesStore {
  roles = $state.raw<MentionRole[]>([]);
  status = $state<MentionRolesStatus>('idle');

  readonly #api: MentionRoleAPI;
  #loadPromise: Promise<boolean> | null = null;

  constructor(api: MentionRoleAPI) {
    this.#api = api;
  }

  /** Ensure the catalogue has loaded, coalescing concurrent consumers. */
  load(): Promise<boolean> {
    if (this.status === 'ready') return Promise.resolve(true);
    return this.refresh();
  }

  /** Reload the catalogue while coalescing with any request already in flight. */
  refresh(): Promise<boolean> {
    if (this.#loadPromise) return this.#loadPromise;

    this.status = 'loading';
    const request = this.#api
      .listRoles()
      .then(({ roles }) => {
        this.roles = roles
          .filter(({ name }) => name !== 'everyone')
          .map(({ name, isSystem, position, pingable }) => ({
            name,
            isSystem,
            position,
            pingable
          }));
        this.status = 'ready';
        return true;
      })
      .catch(() => {
        this.roles = [];
        this.status = 'failed';
        return false;
      })
      .finally(() => {
        if (this.#loadPromise === request) this.#loadPromise = null;
      });

    this.#loadPromise = request;
    return request;
  }
}
