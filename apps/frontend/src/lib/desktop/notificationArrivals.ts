/** Recognize new occurrences in existing resource reads without replaying history or pagination. */
export class NotificationArrivals {
  #primed = false;
  #latest = 0;
  #seen = new Set<string>();

  /** The first successful snapshot is a baseline. Older pages never become new arrivals. */
  read<T extends { id: string; createdAt: string }>(rows: readonly T[]): T[] {
    const arrivals: T[] = [];
    let latest = this.#latest;
    for (const row of rows) {
      const createdAt = Date.parse(row.createdAt);
      if (!Number.isFinite(createdAt)) continue;
      if (this.#primed && createdAt >= this.#latest && !this.#seen.has(row.id)) arrivals.push(row);
      latest = Math.max(latest, createdAt);
    }
    // Retain only IDs at the newest timestamp, including ties absent from this page.
    if (latest > this.#latest) this.#seen.clear();
    for (const row of rows) if (Date.parse(row.createdAt) === latest) this.#seen.add(row.id);
    this.#latest = latest;
    this.#primed = true;
    if (this.#seen.size > 512) this.#seen = new Set([...this.#seen].slice(-512));
    return arrivals;
  }

  /** A privacy reset establishes a new baseline after hydration. */
  reset(): void {
    this.#primed = false;
    this.#latest = 0;
    this.#seen.clear();
  }
}
