import { afterEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import '../../../app.css';
import NativeScreenShareDialog from './NativeScreenShareDialog.svelte';

afterEach(() => vi.restoreAllMocks());

const sources = [
  {
    id: 'window:42',
    kind: 'window' as const,
    applicationName: 'Example Game',
    bundleIdentifier: 'example.game',
    title: 'Main Menu',
    width: 1920,
    height: 1080,
    preview: new Uint8Array()
  },
  {
    id: 'display:1',
    kind: 'display' as const,
    displayIndex: 1,
    isMainDisplay: true,
    width: 2560,
    height: 1440,
    preview: new Uint8Array()
  }
];

describe('NativeScreenShareDialog', () => {
  it('presents application windows and reports an explicit selection', async () => {
    const onselect = vi.fn();
    const ondismiss = vi.fn();
    const { getByRole, getByText } = render(NativeScreenShareDialog, {
      props: {
        visible: true,
        sources,
        loading: false,
        failed: false,
        onretry: vi.fn(),
        onselect,
        ondismiss
      }
    });

    await expect.element(getByRole('heading', { name: 'Share screen' })).toBeInTheDocument();
    await expect.element(getByText('Example Game')).toBeInTheDocument();
    await expect.element(getByText('Main Menu')).toBeInTheDocument();
    await expect.element(getByText('1920×1080')).toBeInTheDocument();

    await userEvent.click(getByRole('button', { name: /Main Menu/ }));

    expect(onselect).toHaveBeenCalledWith(sources[0]);
    await expect.element(getByRole('heading', { name: 'Share screen' })).not.toBeInTheDocument();
    expect(ondismiss).not.toHaveBeenCalled();
  });

  it('switches to full displays and marks them as video-only', async () => {
    const { getByRole, getByText } = render(NativeScreenShareDialog, {
      props: {
        visible: true,
        sources,
        loading: false,
        failed: false,
        onretry: vi.fn(),
        onselect: vi.fn()
      }
    });

    await userEvent.click(getByRole('radio', { name: 'Entire screen' }));

    await expect.element(getByText('Main display')).toBeInTheDocument();
    await expect.element(getByText('Video only')).toBeInTheDocument();
    await expect.element(getByText('2560×1440')).toBeInTheDocument();
    await expect.element(getByText('Example Game')).not.toBeInTheDocument();
  });

  it('offers a retry when source enumeration fails', async () => {
    const onretry = vi.fn();
    const { getByRole } = render(NativeScreenShareDialog, {
      props: {
        visible: true,
        sources: [],
        loading: false,
        failed: true,
        onretry,
        onselect: vi.fn()
      }
    });

    await userEvent.click(getByRole('button', { name: 'Try Again' }));

    expect(onretry).toHaveBeenCalledOnce();
  });

  it('releases in-memory preview URLs when dismissed', async () => {
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:preview');
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const previewSources = [{ ...sources[0], preview: new Uint8Array([0xff, 0xd8, 0xff]) }];
    const { getByRole } = render(NativeScreenShareDialog, {
      props: {
        visible: true,
        sources: previewSources,
        loading: false,
        failed: false,
        onretry: vi.fn(),
        onselect: vi.fn()
      }
    });

    await expect.poll(() => createObjectURL.mock.calls.length).toBe(1);
    await userEvent.click(getByRole('button', { name: 'Cancel' }));

    await expect.poll(() => revokeObjectURL.mock.calls).toContainEqual(['blob:preview']);
  });
});
