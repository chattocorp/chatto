import { expect, it } from 'vitest';
import { effect } from '../reactivity/index.js';
import { assetUrlForServer } from '../util/assetUrls.js';
import { createClient } from '../client.js';

it('updates asset URLs that were read before their server registered', () => {
  const client = createClient();
  const urls: (string | null)[] = [];
  const stop = effect(() => {
    urls.push(assetUrlForServer('late-server', '/assets/files/one'));
  });
  client.registry.addServer({
    id: 'late-server',
    url: 'https://late.example',
    name: 'Late',
    iconUrl: null,
    addedAt: 1
  });
  stop();
  client.close();
  expect(urls).toEqual(['/assets/files/one', 'https://late.example/assets/files/one']);
});
