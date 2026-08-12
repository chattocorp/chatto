import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import '../../../app.css';
import GameCaptureSourceDialog from './GameCaptureSourceDialog.svelte';

const sources = [
  {
    id: 'window:42',
    kind: 'window' as const,
    applicationName: 'Example Game',
    bundleIdentifier: 'example.game',
    title: 'Main Menu',
    width: 1920,
    height: 1080
  }
];

describe('GameCaptureSourceDialog', () => {
  it('presents native windows and reports an explicit selection', async () => {
    const onselect = vi.fn();
    const { getByRole, getByText } = render(GameCaptureSourceDialog, {
      props: {
        visible: true,
        sources,
        loading: false,
        failed: false,
        onretry: vi.fn(),
        onselect
      }
    });

    await expect.element(getByRole('heading', { name: 'Stream a game' })).toBeInTheDocument();
    await expect.element(getByText('Example Game')).toBeInTheDocument();
    await expect.element(getByText('Main Menu')).toBeInTheDocument();
    await expect.element(getByText('1920×1080')).toBeInTheDocument();

    await userEvent.click(getByRole('button', { name: /Main Menu/ }));

    expect(onselect).toHaveBeenCalledWith(sources[0]);
  });

  it('offers a retry when source enumeration fails', async () => {
    const onretry = vi.fn();
    const { getByRole } = render(GameCaptureSourceDialog, {
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

  it('filters grouped windows by application and title', async () => {
    const moonring = {
      ...sources[0],
      id: 'window:99',
      applicationName: 'Moonring',
      bundleIdentifier: 'com.fluttermind.moonring',
      title: 'Moonring'
    };
    const { getByRole } = render(GameCaptureSourceDialog, {
      props: {
        visible: true,
        sources: [...sources, moonring],
        loading: false,
        failed: false,
        onretry: vi.fn(),
        onselect: vi.fn()
      }
    });

    await userEvent.fill(getByRole('textbox', { name: 'Search' }), 'moon');

    await expect.element(getByRole('heading', { name: 'Moonring' })).toBeInTheDocument();
    await expect
      .element(getByRole('heading', { name: 'Example Game' }))
      .not.toBeInTheDocument();
  });
});
