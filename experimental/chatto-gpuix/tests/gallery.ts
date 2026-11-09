/** Check reusable native navigation, disclosure, buttons and composer through GPUI hit testing. */
import {
  all_text,
  bounds,
  check,
  click_test_id,
  click_text,
  find_test_id,
  finish,
  focus,
  mount_headless,
  screenshot,
  settle,
  type
} from 'gpuix-svelte/test';
import { changeScale } from '../src/ui/scale.svelte.ts';
import { applyTheme } from '../src/ui/theme.ts';
import Gallery from '../src/Gallery.svelte';

const mounted = mount_headless(Gallery, { width: 1100, height: 538 });
try {
  if (process.env.CHATTO_GPUIX_SCREENSHOT) screenshot(process.env.CHATTO_GPUIX_SCREENSHOT);
  check('gallery starts with the selected channel', all_text().includes('chatto-development'));
  const sidebar = bounds(find_test_id('gallery-server-header')!)!;
  const channel = bounds(find_test_id('gallery-channel-header')!)!;
  check(
    'sidebar and conversation headers align',
    Math.abs(sidebar.y - channel.y) < 0.1 && Math.abs(sidebar.height - channel.height) < 0.1
  );
  changeScale(1.37);
  applyTheme();
  settle();
  const scaledSidebar = bounds(find_test_id('gallery-server-header')!)!;
  const scaledChannel = bounds(find_test_id('gallery-channel-header')!)!;
  check(
    'pane headers stay aligned at arbitrary UI scales',
    Math.abs(scaledSidebar.height - sidebar.height * 1.37) < 0.1 &&
      Math.abs(scaledSidebar.height - scaledChannel.height) < 0.1 &&
      Math.abs(scaledSidebar.y - scaledChannel.y) < 0.1
  );
  changeScale(1);
  applyTheme();
  settle();
  click_test_id('gallery-bugs');
  check('channel component changes selection', all_text().includes('bug-reports'));
  click_text('CHATTO');
  check('sidebar section collapses its channels', find_test_id('gallery-channel'), null);
  click_text('CHATTO');
  check('sidebar section expands its channels', !!find_test_id('gallery-channel'));
  const input = find_test_id('live-draft');
  if (!input) throw new Error('Missing shared composer');
  focus(input);
  type('g a l l e r y space m e s s a g e');
  click_test_id('live-send');
  check('shared composer delivers its draft', all_text().includes('gallery message'));
  const primary = find_test_id('gallery-primary');
  if (!primary) throw new Error('Missing primary button');
  mounted.native.scrollIntoView(primary.id);
  settle();
  click_test_id('gallery-primary');
  check('shared button dispatches its action', all_text().includes('Primary action selected'));
} finally {
  await mounted.unmount();
}
finish('Chatto native gallery', 8);
