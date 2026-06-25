import { describe, it, expect } from 'vitest';
import { render } from 'vitest-browser-svelte';
import DropZoneOverlay from './DropZoneOverlay.svelte';

describe('DropZoneOverlay', () => {
  it('renders nothing when hidden', () => {
    const { container } = render(DropZoneOverlay, {
      props: { visible: false }
    });

    expect(container.textContent).not.toContain('Drop files here');
  });

  it('renders the default drop copy when visible', async () => {
    const { getByText } = render(DropZoneOverlay, {
      props: { visible: true }
    });

    await expect.element(getByText('Drop files here')).toBeInTheDocument();
    await expect
      .element(getByText('Images and videos will be added to your message'))
      .toBeInTheDocument();
  });

  it('renders custom drop copy', async () => {
    const { getByText } = render(DropZoneOverlay, {
      props: {
        visible: true,
        title: 'Drop image',
        subtitle: 'Upload as your avatar'
      }
    });

    await expect.element(getByText('Drop image')).toBeInTheDocument();
    await expect.element(getByText('Upload as your avatar')).toBeInTheDocument();
  });
});
