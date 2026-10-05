import { expect, it } from 'vitest';
import { effect } from '../reactivity/index.js';
import { ServerCatalog } from './catalog.js';

const registration = (id: string) => ({
  id,
  url: `https://${id}.example`,
  name: id,
  iconUrl: null,
  addedAt: 1
});

it('publishes new registrations for every change', () => {
  const catalogBefore = new ServerCatalog([registration('solo')]);
  const before = catalogBefore.get('solo');
  catalogBefore.update('solo', { name: 'Renamed' });
  expect(catalogBefore.get('solo')).not.toBe(before);
  expect(catalogBefore.get('solo')?.name).toBe('Renamed');
});

it('notifies readers when a server is added, updated, or removed', () => {
  const catalog = new ServerCatalog([registration('one')]);
  const runs: string[][] = [];
  const stop = effect(() => {
    runs.push(catalog.registrations.map((entry) => entry.name));
  });

  catalog.add(registration('two'));
  catalog.update('one', { name: 'Renamed' });
  catalog.remove('two');

  expect(runs).toEqual([['one'], ['one', 'two'], ['Renamed', 'two'], ['Renamed']]);
  stop();
});
