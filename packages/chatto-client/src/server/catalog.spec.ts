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

it('notifies readers when a server is added, updated, or removed', () => {
  const catalog = new ServerCatalog([registration('one')]);
  const runs: string[][] = [];
  const stop = effect(() => {
    runs.push(catalog.registrations.map((entry) => entry.name));
  });
  const retained = catalog.get('one');

  catalog.add(registration('two'));
  catalog.update('one', { name: 'Renamed' });
  catalog.remove('two');

  expect(runs).toEqual([['one'], ['one', 'two'], ['Renamed', 'two'], ['Renamed']]);
  expect(catalog.get('one')).toBe(retained);
  stop();
});
