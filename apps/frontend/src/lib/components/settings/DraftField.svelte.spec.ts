import { flushSync } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { DraftField } from './DraftField.svelte';

let dispose: (() => void) | undefined;
afterEach(() => dispose?.());

describe('DraftField', () => {
  it('adopts pristine fields and preserves dirty fields across remote refreshes', () => {
    let remote = $state({ name: 'Original', description: 'Before' });
    let name!: DraftField<string>;
    let description!: DraftField<string>;
    dispose = $effect.root(() => {
      name = new DraftField(() => remote.name);
      description = new DraftField(() => remote.description);
    });
    flushSync();
    name.value = 'Local';
    flushSync(() => {
      remote = { name: 'Remote', description: 'After' };
    });
    expect(name.value).toBe('Local');
    expect(name.original).toBe('Remote');
    expect(description.value).toBe('After');
    expect(description.original).toBe('After');

    // Returning to the latest remote value makes this field pristine again.
    name.value = 'Remote';
    flushSync(() => {
      remote.name = 'Later';
    });
    expect(name.value).toBe('Later');
  });

  it('preserves raw text edits and handles numeric and boolean fields', () => {
    let remote = $state({ name: 'Name', interval: 10, enabled: false });
    let name!: DraftField<string>;
    let interval!: DraftField<number>;
    let enabled!: DraftField<boolean>;
    dispose = $effect.root(() => {
      name = new DraftField(() => remote.name);
      interval = new DraftField(() => remote.interval);
      enabled = new DraftField(() => remote.enabled);
    });
    flushSync();
    name.value = 'Name ';
    interval.value = 20;
    flushSync(() => {
      remote = { name: 'Renamed', interval: 30, enabled: true };
    });
    expect(name.value).toBe('Name ');
    expect(interval.value).toBe(20);
    expect(interval.original).toBe(30);
    expect(enabled.value).toBe(true);

    dispose?.();
    flushSync(() => {
      remote.enabled = false;
    });
    expect(enabled.value).toBe(true);
    dispose = undefined;
  });
});
