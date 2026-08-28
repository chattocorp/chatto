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
 * Updates preserve registration object identity. Removal and reset deliberately
 * invalidate retained entries at their lifecycle boundary.
 */
export class ServerCatalog {
  registrations = $state<ServerRegistration[]>([]);

  constructor(initial: ServerRegistration[] = []) {
    this.registrations = initial.map((registration) => ({ ...registration }));
  }

  get(id: string): ServerRegistration | undefined {
    return this.registrations.find((registration) => registration.id === id);
  }

  add(registration: ServerRegistration): boolean {
    if (this.get(registration.id)) return false;
    this.registrations.push({ ...registration });
    return true;
  }

  update(id: string, data: ServerRegistrationMetadataPatch): boolean {
    const registration = this.get(id);
    if (!registration) return false;
    Object.assign(registration, data);
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
