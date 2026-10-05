import type { RoleAPI } from '../api/roles.js';
import { signal } from '../reactivity/index.js';
import type { MentionRole } from '../room/mentionRoles.js';

type MentionRoleAPI = Pick<RoleAPI, 'listRoles'>;

export type MentionRolesStatus = 'idle' | 'loading' | 'ready' | 'failed';

/** Shared public role catalogue used by message rendering and composers. */
export class MentionRolesStore {
  readonly #rolesSignal = signal<MentionRole[]>([]);
  get roles(): MentionRole[] {
    return this.#rolesSignal.get();
  }
  set roles(value: MentionRole[]) {
    this.#rolesSignal.set(value);
  }
  readonly #statusSignal = signal<MentionRolesStatus>('idle');
  get status(): MentionRolesStatus {
    return this.#statusSignal.get();
  }
  set status(value: MentionRolesStatus) {
    this.#statusSignal.set(value);
  }

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
