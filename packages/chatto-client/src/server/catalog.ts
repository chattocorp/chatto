import { signal } from '../reactivity/index.js';

/** Public metadata for one Chatto server known to this client. */
export interface ServerRegistration {
  id: string;
  url: string;
  name: string;
  iconUrl: string | null;
  addedAt: number;
}

export type ServerRegistrationMetadataPatch = Partial<
  Pick<ServerRegistration, 'name' | 'iconUrl' | 'addedAt'>
>;

/**
 * Owns the server catalogue independently from device-local authentication.
 *
 * Every change publishes new registration objects and a new array, so
 * reactive readers see it. Do not retain a registration object; read it again
 * with `get`.
 */
export class ServerCatalog {
  readonly #registrationsSignal = signal<ServerRegistration[]>([]);
  get registrations(): ServerRegistration[] {
    return this.#registrationsSignal.get();
  }
  set registrations(value: ServerRegistration[]) {
    this.#registrationsSignal.set(value);
  }

  constructor(initial: ServerRegistration[] = []) {
    this.registrations = initial.map((registration) => ({ ...registration }));
  }

  get(id: string): ServerRegistration | undefined {
    return this.registrations.find((registration) => registration.id === id);
  }

  add(registration: ServerRegistration): boolean {
    if (this.get(registration.id)) return false;
    this.registrations = [...this.registrations, { ...registration }];
    return true;
  }

  update(id: string, data: ServerRegistrationMetadataPatch): boolean {
    const index = this.registrations.findIndex((registration) => registration.id === id);
    if (index === -1) return false;
    this.registrations = this.registrations.with(index, { ...this.registrations[index], ...data });
    return true;
  }

  remove(id: string): boolean {
    if (!this.get(id)) return false;
    this.registrations = this.registrations.filter((registration) => registration.id !== id);
    return true;
  }

  reset(registrations: ServerRegistration[] = []): void {
    this.registrations = registrations.map((registration) => ({ ...registration }));
  }
}
