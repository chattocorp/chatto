import { SvelteSet } from 'svelte/reactivity';

export type ServerRegistrationSource = 'local' | 'synced';

/** Public metadata for one Chatto server known to this client. */
export interface ServerRegistration {
  id: string;
  url: string;
  name: string;
  iconUrl: string | null;
  addedAt: number;
  /** Local provenance used to prevent data from leaking across Authling accounts. */
  source: ServerRegistrationSource;
}

export type ServerRegistrationMetadataPatch = Partial<
  Pick<ServerRegistration, 'name' | 'iconUrl' | 'addedAt'>
>;

export type ServerCatalogChange = 'public' | 'local-reset';

/**
 * Owns the server catalogue independently from device-local authentication.
 *
 * Updates preserve registration object identity. Removal and reset deliberately
 * invalidate retained entries at their lifecycle boundary.
 */
export class ServerCatalog {
  registrations = $state<ServerRegistration[]>([]);
  #listeners = new SvelteSet<(change: ServerCatalogChange) => void>();

  constructor(initial: ServerRegistration[] = []) {
    this.registrations = initial.map((registration) => ({ ...registration }));
  }

  get(id: string): ServerRegistration | undefined {
    return this.registrations.find((registration) => registration.id === id);
  }

  add(registration: ServerRegistration): boolean {
    if (this.get(registration.id)) return false;
    this.registrations.push({ ...registration });
    this.#notify('public');
    return true;
  }

  update(id: string, data: ServerRegistrationMetadataPatch): boolean {
    const registration = this.get(id);
    if (!registration) return false;
    Object.assign(registration, data);
    this.#notify('public');
    return true;
  }

  /** Promote synchronized provenance without changing the server identity or origin. */
  markLocal(id: string): boolean {
    const registration = this.get(id);
    if (!registration) return false;
    registration.source = 'local';
    this.#notify('public');
    return true;
  }

  remove(id: string): boolean {
    if (!this.get(id)) return false;
    this.registrations = this.registrations.filter((registration) => registration.id !== id);
    this.#notify('public');
    return true;
  }

  reset(registrations: ServerRegistration[] = []): void {
    this.registrations = registrations.map((registration) => ({ ...registration }));
    this.#notify('local-reset');
  }

  subscribe(listener: (change: ServerCatalogChange) => void): () => void {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }

  #notify(change: ServerCatalogChange): void {
    for (const listener of this.#listeners) listener(change);
  }
}
