import type { ComponentProps } from 'svelte';
import { render } from 'vitest-browser-svelte';
import VoiceCallPanelStoryHarness from './VoiceCallPanelStoryHarness.svelte';

type HarnessProps = ComponentProps<typeof VoiceCallPanelStoryHarness>;

/**
 * Renders the call panel harness for a spec. Stage layouts, the harness
 * default, get a fixed pane size as in the app: the stage measures itself to
 * size and place the featured card, so an unsized stage would follow its own
 * content and resize in a loop.
 */
export function renderCallPanelHarness(props: HarnessProps = {}) {
  const screen = render(VoiceCallPanelStoryHarness, { props });
  if (props.playableMedia) {
    Object.assign(screen.container.style, { display: 'flex', flexDirection: 'column' });
  }
  if ((props.layout ?? 'stage') === 'stage') {
    Object.assign(screen.container.style, { display: 'flex', width: '1080px', height: '720px' });
  }
  return screen;
}
