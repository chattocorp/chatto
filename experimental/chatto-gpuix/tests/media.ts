import { fixturePng } from './imageFixture.ts';
/** Native image lifecycle and social previews use synthetic bytes, without external requests. */
import {
  all_text,
  check,
  click_test_id,
  find_test_id,
  finish,
  mount_headless,
  settle,
  wait_for
} from 'gpuix-svelte/test';
import ThemeHarness from './ThemeHarness.svelte';
import MediaHarness from './MediaHarness.svelte';
import SocialPreview from '../src/SocialPreview.svelte';
import LinkPreview from '../src/LinkPreview.svelte';
import { mediaUrl, readImage } from '../src/media.ts';

const png = fixturePng;
const originalFetch = globalThis.fetch;
let requests = 0;
let lastOptions: RequestInit | undefined;
let complete: (response: Response) => void = () => {};
let pending = false;
let fail = false;
let requestSignal: AbortSignal | null | undefined;
globalThis.fetch = async (_url, options) => {
  requests++;
  lastOptions = options;
  requestSignal = options?.signal;
  if (pending) return new Promise<Response>((resolve) => (complete = resolve));
  return fail
    ? new Response('<html>not an image</html>', { headers: { 'content-type': 'text/html' } })
    : imageResponse();
};
function imageResponse() {
  return new Response(png, { headers: { 'content-type': 'image/png' } });
}

try {
  check(
    'non-web and credential-bearing URLs are rejected',
    mediaUrl('file:///etc/passwd', 'https://chat.example.test') === null &&
      mediaUrl('https://user:secret@example.test/image', 'https://chat.example.test') === null
  );
  check(
    'relative attachment URLs use the selected server',
    mediaUrl('/assets/files/fixture', 'https://chat.example.test')?.origin,
    'https://chat.example.test'
  );
  const mounted = mount_headless(MediaHarness, { width: 500, height: 400 });
  try {
    check('offscreen external image sends no request', requests, 0);
    click_test_id('load-external-image');
    check('consent alone does not fetch an offscreen image', requests, 0);
    mounted.component.setActive(true);
    await wait_for(() => !!find_test_id('media-image'), { label: 'visible image' });
    check('visible image loads after consent', requests, 1);
    check(
      'image transport omits cookies, referrers and redirects',
      lastOptions?.credentials === 'omit' &&
        lastOptions.redirect === 'error' &&
        lastOptions.referrerPolicy === 'no-referrer'
    );
    mounted.component.setActive(false);
    settle();
    check('leaving the viewport releases image data', find_test_id('media-image'), null);
    pending = true;
    mounted.component.setActive(true);
    settle();
    await wait_for(() => requests === 2);
    mounted.component.setActive(false);
    settle();
    check('leaving the viewport cancels the pending request', requestSignal?.aborted, true);
    complete(imageResponse());
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    settle();
    check('late image response cannot revive an offscreen row', find_test_id('media-image'), null);
    pending = false;
    fail = true;
    mounted.component.setSource('https://chat.example.test/failed.png');
    mounted.component.setActive(true);
    await wait_for(() => all_text().some((text) => text.includes('Could not load image')));
    check('unsupported content has a visible retry', !!find_test_id('retry-image'));
    fail = false;
    click_test_id('retry-image');
    await wait_for(() => !!find_test_id('media-image'));
    check('image retry renders recovered content', !!find_test_id('media-image'));
    const beforeReplacement = requests;
    mounted.component.setSource('https://other.example.test/replacement.png');
    settle();
    check(
      'a replacement external image requires new consent',
      requests === beforeReplacement && !!find_test_id('load-external-image')
    );
  } finally {
    await mounted.unmount();
  }

  const beforeSocial = requests;
  const social = mount_headless(ThemeHarness, {
    width: 500,
    height: 500,
    props: {
      component: SocialPreview,
      componentProps: {
        serverUrl: 'https://chat.example.test',
        active: true,
        post: {
          provider: 'bluesky',
          author: { displayName: 'Fixture author', handle: 'fixture.test' },
          text: 'Concealed fixture text',
          contentWarning: 'Fixture warning',
          images: [{ url: 'https://images.example.test/social.png' }]
        }
      }
    }
  });
  try {
    check(
      'social content warning conceals the body',
      !all_text().includes('Concealed fixture text')
    );
    check('concealed post sends no media request', requests, beforeSocial);
    click_test_id('reveal-social');
    check('social content can be revealed', all_text().includes('Concealed fixture text'));
    check('revealing text still requires external image consent', requests, beforeSocial);
  } finally {
    await social.unmount();
  }

  const card = mount_headless(ThemeHarness, {
    width: 500,
    height: 500,
    props: {
      component: LinkPreview,
      componentProps: {
        serverUrl: 'https://chat.example.test',
        active: true,
        preview: {
          url: 'https://example.test/post',
          title: 'Fixture preview title',
          description: 'Fixture description',
          siteName: 'Fixture site'
        }
      }
    }
  });
  try {
    check(
      'link card renders SDK metadata without a browser',
      all_text().includes('Fixture preview title') && all_text().includes('Fixture description')
    );
  } finally {
    await card.unmount();
  }

  globalThis.fetch = async () =>
    new Response(new Uint8Array(12 * 1024 * 1024 + 1), {
      headers: { 'content-type': 'image/png' }
    });
  let rejected = false;
  try {
    await readImage(new URL('https://example.test/large.png'), new AbortController().signal);
  } catch {
    rejected = true;
  }
  check('oversized previews are rejected', rejected);
} finally {
  globalThis.fetch = originalFetch;
}
finish('Chatto GPUix media', 18);
