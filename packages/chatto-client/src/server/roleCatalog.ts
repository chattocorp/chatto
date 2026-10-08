import type { RoleAPI } from '../api/roles.js';
import { computed, signal } from '../reactivity/index.js';
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
  if (roleName === EVERYONE) return true;
  if (roleName === OWNER) return false;
  if (highestRole === OWNER) return true;
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
 * and accounts the role hierarchy lets the viewer manage. The server still
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
  readonly #viewerHighestRoleSignal = signal(EVERYONE);
  /**
   * The role at which the viewer ranks: `owner` for owners of the server,
   * otherwise the viewer's highest role, or `everyone` without roles.
   */
  get viewerHighestRole(): string {
    return this.#viewerHighestRoleSignal.get();
  }
  set viewerHighestRole(value: string) {
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

  /** Whether the role hierarchy lets the viewer manage `roleName`. */
  ranksBelowViewer(roleName: string): boolean {
    return roleRanksBelow(this.#order.get(), this.viewerHighestRole, roleName);
  }

  /** Whether the role hierarchy lets the viewer act on an account with `accountRoles`. */
  viewerOutranks(accountRoles: readonly string[]): boolean {
    return outranksAccount(this.#order.get(), this.viewerHighestRole, accountRoles);
  }

  readonly #order = computed(() => this.roles.map((role) => role.name));

  /** Discard obsolete reads so an event can request a fresh catalogue. */
  invalidate(): void {
    this.#generation++;
    this.#loadPromise = null;
    this.roles = [];
    this.viewerHighestRole = EVERYONE;
    this.status = 'idle';
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
        this.roles = roles
          .filter(({ name }) => name !== EVERYONE)
          .map(({ name, isSystem, pingable }) => ({ name, isSystem, pingable }));
        this.viewerHighestRole = viewerHighestRole;
        this.status = 'ready';
        return true;
      })
      .catch(() => {
        if (generation !== this.#generation) return false;
        this.roles = [];
        this.viewerHighestRole = EVERYONE;
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
