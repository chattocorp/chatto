/** Exercise real keyboard delivery, scaled hitboxes, arbitrary percentages, and saved appearance. */
import {
  bounds,
  check,
  click_test_id,
  find_test_id,
  finish,
  focus,
  mount_headless,
  press,
  screenshot,
  settle,
  type
} from 'gpuix-svelte/test';
import App from '../src/App.svelte';
import { NativeSession } from '../src/session.svelte.ts';
import { memoryState } from '../src/state.ts';
import { appearance } from '../src/ui/scale.svelte.ts';
const state = memoryState();
const session = new NativeSession({ openState: async () => state });
await session.ready();
const mounted = mount_headless(App, { props: { session }, width: 1100, height: 538 });
try {
  const original = bounds(find_test_id('add-server')!)!;
  press('cmd-=');
  check('window shortcut increases scale', Math.round(appearance.scale * 100), 110);
  check(
    'zoom scales the native button hitbox',
    Math.abs(bounds(find_test_id('add-server')!)!.width / original.width - 1.1) < 0.02
  );
  press('cmd--');
  check('window shortcut decreases scale', Math.round(appearance.scale * 100), 100);
  click_test_id('manage-servers');
  const input = find_test_id('scale-percentage')!;
  focus(input);
  press('cmd-a');
  type('1 3 7');
  press('enter');
  check('percentage field accepts arbitrary scale', Math.round(appearance.scale * 100), 137);
  check('scale is saved in host UI state', state.ui.uiScale, 1.37);
  session.select('fixture-room');
  check('other UI writes retain scale', state.ui.uiScale, 1.37);
  press('cmd-0');
  check('reset shortcut works while an input has focus', appearance.scale, 1);
  click_test_id('scale-larger');
  check('larger control uses native hit testing', Math.round(appearance.scale * 100), 110);
  click_test_id('scale-reset');
  check('reset control returns to 100 percent', appearance.scale, 1);
  session.setUiScale(9);
  settle();
  check('scale has an upper bound', appearance.scale, 2);
  session.setUiScale(0.01);
  settle();
  check('scale has a lower bound', appearance.scale, 0.5);
  session.setUiScale(1.25);
  settle();
  if (process.env.CHATTO_GPUIX_SCREENSHOT) screenshot(process.env.CHATTO_GPUIX_SCREENSHOT);
} finally {
  await mounted.unmount();
  await session.close();
}
const saved = memoryState();
saved.setUi({ activeServerId: '', rooms: {}, order: [], uiScale: 1.37 });
const restored = new NativeSession({ openState: async () => saved });
await restored.ready();
check('session initialization restores saved scale', appearance.scale, 1.37);
await restored.close();
finish('Chatto GPUix scale', 12);
