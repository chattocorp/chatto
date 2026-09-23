import type { RoleAPI } from '$lib/api-client/roles';
import type { MentionRole } from '$lib/state/room';

type MentionRoleAPI = Pick<RoleAPI, 'listRoles'>;

export type MentionRolesStatus = 'idle' | 'loading' | 'ready' | 'failed';

/** Shared public role catalogue used by message rendering and composers. */
export class MentionRolesStore {
  roles = $state.raw<MentionRole[]>([]);
  status = $state<MentionRolesStatus>('idle');

  readonly #api: MentionRoleAPI;
  readonly #canLoad: () => boolean;
  #loadPromise: Promise<boolean> | null = null;
  #generation = 0;

  /** Discard obsolete reads so an event can request a fresh catalogue. */
  invalidate(): void {
    this.#generation++;
    this.#loadPromise = null;
    this.roles = [];
    this.status = 'idle';
  }

  constructor(api: MentionRoleAPI, canLoad: () => boolean = () => true) {
    this.#api = api;
    this.#canLoad = canLoad;
  }

  /** Ensure the catalogue has loaded, coalescing concurrent consumers. */
  load(): Promise<boolean> {
    if (!this.#canLoad()) return Promise.resolve(false);
    if (this.status === 'ready') return Promise.resolve(true);
    return this.refresh();
  }

  /** Reload the catalogue while coalescing with any request already in flight. */
  refresh(): Promise<boolean> {
    if (!this.#canLoad()) return Promise.resolve(false);
    if (this.#loadPromise) return this.#loadPromise;

    this.status = 'loading';
    const generation = this.#generation;
    const request = this.#api
      .listRoles()
      .then(({ roles }) => {
        if (generation !== this.#generation) return false;
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
        if (generation !== this.#generation) return false;
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
