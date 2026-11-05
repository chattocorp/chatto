/** Exercise the app through native hit testing, without a browser or a server. */
import {
  all_text,
  check,
  click_test_id,
  find_test_id,
  finish,
  focus,
  mount_headless,
  screenshot,
  type
} from 'gpuix-svelte/test';
import App from '../src/Demo.svelte';

mount_headless(App, { width: 1100, height: 500 });
check('starts with the general history', all_text().includes('80 messages'));
click_test_id('add-samples');
check('the sample action updates the virtual history', all_text().includes('180 messages'));
click_test_id('room-playground');
check('channel selection changes the timeline', all_text().includes('8 messages'));
const input = find_test_id('draft');
if (!input) throw new Error('The composer was not mounted');
focus(input);
type('h e l l o');
click_test_id('send');
check('local composer adds a message', all_text().includes('hello'));
check('the room history count changes', all_text().includes('9 messages'));
click_test_id('room-general');
check('channel histories stay independent', all_text().includes('180 messages'));
if (process.env.CHATTO_GPUIX_SCREENSHOT) screenshot(process.env.CHATTO_GPUIX_SCREENSHOT);
finish('Chatto GPUix', 6);
