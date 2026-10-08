import type { RoleAPI } from '../api/roles.js';
import { batch, computed, signal } from '../reactivity/index.js';
import type { MentionRole } from '../room/mentionRoles.js';

type RoleCatalogAPI = Pick<RoleAPI, 'listRoles'>;

export type RoleCatalogStatus = 'idle' | 'loading' | 'ready' | 'failed';

const OWNER = 'owner';
const EVERYONE = 'everyone';

/**
 * Reports whether the role hierarchy lets an account that ranks at
 * `highestRole` manage the role `roleName` (ADR-114). `order` lists role names
 * highest first and may leave out `everyone`, which ranks below every account
 * and is always manageable. Only owners manage `owner`.
 */
export function roleRanksBelow(
  order: readonly string[],
  highestRole: string,
  roleName: string
): boolean {
  if (roleName === EVERYONE || highestRole === OWNER) return true;
  if (roleName === OWNER) return false;
  const highest = order.indexOf(highestRole);
  return highest >= 0 && order.indexOf(roleName) > highest;
}

/**
 * Reports whether an account that ranks at `highestRole` outranks an account
 * that holds `accountRoles` (ADR-114). Owners outrank everybody else. Other
 * accounts must rank strictly higher, so two accounts with the same highest
 * role, or two accounts without roles, do not outrank each other.
 */
export function outranksAccount(
  order: readonly string[],
  highestRole: string,
  accountRoles: readonly string[]
): boolean {
  if (highestRole === OWNER) return true;
  if (accountRoles.includes(OWNER)) return false;
  const rank = (roleName: string) =>
    roleName === EVERYONE ? order.length : order.indexOf(roleName);
  const viewer = rank(highestRole);
  const account = Math.min(order.length, ...accountRoles.map(rank).filter((index) => index >= 0));
  return viewer >= 0 && viewer < account;
}

/**
 * Shared public role catalogue: the roles in role order, highest first, and
 * the role at which the viewer ranks. Message rendering and composers use its
 * roles as mention targets. Management screens use it to show which roles
 * and accounts the role hierarchy lets the viewer manage. While the viewer's
 * rank is unknown, the hierarchy checks allow everything: the server still
 * checks the hierarchy and the permission for every action.
 */
export class RoleCatalogStore {
  readonly #rolesSignal = signal<MentionRole[]>([]);
  /** Roles except `everyone`, highest first. */
  get roles(): MentionRole[] {
    return this.#rolesSignal.get();
  }
  set roles(value: MentionRole[]) {
    this.#rolesSignal.set(value);
  }
  readonly #viewerHighestRoleSignal = signal<string | null>(null);
  /**
   * The role at which the viewer ranks: `owner` for owners of the server,
   * otherwise the viewer's highest role, or `everyone` without roles. `null`
   * until the catalogue loads, and after a reset or a failed read.
   */
  get viewerHighestRole(): string | null {
    return this.#viewerHighestRoleSignal.get();
  }
  set viewerHighestRole(value: string | null) {
    this.#viewerHighestRoleSignal.set(value);
  }
  readonly #statusSignal = signal<RoleCatalogStatus>('idle');
  get status(): RoleCatalogStatus {
    return this.#statusSignal.get();
  }
  set status(value: RoleCatalogStatus) {
    this.#statusSignal.set(value);
  }

  readonly #api: RoleCatalogAPI;
  readonly #canLoad: () => boolean;
  #loadPromise: Promise<boolean> | null = null;
  #generation = 0;

  constructor(api: RoleCatalogAPI, canLoad: () => boolean = () => true) {
    this.#api = api;
    this.#canLoad = canLoad;
  }

  /**
   * Whether the role hierarchy lets the viewer manage `roleName`. True while
   * the viewer's rank is unknown.
   */
  ranksBelowViewer(roleName: string): boolean {
    const highestRole = this.viewerHighestRole;
    return highestRole === null || roleRanksBelow(this.#order.get(), highestRole, roleName);
  }

  /**
   * Whether the role hierarchy lets the viewer act on an account with
   * `accountRoles`. True while the viewer's rank is unknown.
   */
  viewerOutranks(accountRoles: readonly string[]): boolean {
    const highestRole = this.viewerHighestRole;
    return highestRole === null || outranksAccount(this.#order.get(), highestRole, accountRoles);
  }

  readonly #order = computed(() => this.roles.map((role) => role.name));

  /** Discard the catalogue and obsolete reads, for example after a reset. */
  invalidate(): void {
    this.#generation++;
    this.#loadPromise = null;
    batch(() => {
      this.roles = [];
      this.viewerHighestRole = null;
      this.status = 'idle';
    });
  }

  /**
   * Read the catalogue again after a role change. The current catalogue stays
   * until the new read arrives, so screens keep their state meanwhile.
   */
  reload(): Promise<boolean> {
    this.#generation++;
    this.#loadPromise = null;
    return this.refresh();
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
      .then(({ roles, viewerHighestRole }) => {
        if (generation !== this.#generation) return false;
        batch(() => {
          this.roles = roles
            .filter(({ name }) => name !== EVERYONE)
            .map(({ name, isSystem, pingable }) => ({ name, isSystem, pingable }));
          this.viewerHighestRole = viewerHighestRole;
          this.status = 'ready';
        });
        return true;
      })
      .catch(() => {
        if (generation !== this.#generation) return false;
        batch(() => {
          this.roles = [];
          this.viewerHighestRole = null;
          this.status = 'failed';
        });
        return false;
      })
      .finally(() => {
        if (this.#loadPromise === request) this.#loadPromise = null;
      });

    this.#loadPromise = request;
    return request;
  }
}
