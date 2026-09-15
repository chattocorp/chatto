import { expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
import AudioDeviceMenu from './AudioDeviceMenu.svelte';

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    store: {
      voiceCall: {
        preferences: new CallPreferencesState('device-menu-test'),
        microphoneLevel: 0.5,
        microphoneGateUnavailable: false,
        audioDevices: [],
        audioOutputDevices: [],
        videoDevices: [],
        selectedDeviceId: '',
        selectedOutputDeviceId: '',
        selectedVideoDeviceId: ''
      }
    }
  })
}));

it('exposes device controls as a dialog and supports keyboard threshold changes', async () => {
  localStorage.clear();
  const screen = render(AudioDeviceMenu, {
    props: {
      anchor: { top: 100, bottom: 120, left: 100 },
      onclose: vi.fn()
    }
  });
  await expect.element(screen.getByRole('dialog', { name: 'Devices' })).toBeInTheDocument();
  const slider = document.querySelector<HTMLInputElement>('#call-microphone-sensitivity')!;
  slider.focus();
  await userEvent.keyboard('{Home}{ArrowRight}');
  expect(new CallPreferencesState('device-menu-test').microphoneThreshold).toBe(-59);
  await expect
    .element(screen.getByRole('meter', { name: 'Microphone level' }))
    .toHaveAttribute('aria-valuenow', '0.5');
});
