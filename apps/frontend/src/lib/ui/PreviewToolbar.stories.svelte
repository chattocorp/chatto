<script module lang="ts">
  import { expect, userEvent, waitFor } from 'storybook/test';
  import SvgAnimation from './SvgAnimation.svelte';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PreviewToolbar from './PreviewToolbar.svelte';
  import PreviewFrame from './PreviewFrame.svelte';
  import PreviewAction from './PreviewAction.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import SocialPostEmbed from '$lib/components/SocialPostEmbed.svelte';
  import coast from '../../../e2e/fixtures/brighton.jpg?url';

  const animationSource =
    '<svg xmlns="http://www.w3.org/2000/svg" width="256" height="192"><rect width="256" height="192" fill="#164e63"/><circle cx="64" cy="96" r="24" fill="#67e8f9"><animate attributeName="cx" values="64;192;64" dur="3s" repeatCount="indefinite"/></circle></svg>';

  const { Story } = defineMeta({
    title: 'UI/PreviewToolbar',
    component: PreviewToolbar,
    tags: ['autodocs']
  });

  /** Check the shared rail geometry and keyboard access in each preview family. */
  async function checkRails({ canvasElement }: { canvasElement: HTMLElement }) {
    for (const rail of canvasElement.querySelectorAll<HTMLElement>('.preview-toolbar')) {
      const buttons = Array.from(rail.querySelectorAll('button'));
      buttons[0].focus();
      await waitFor(() =>
        expect(rail.closest('.preview-frame-actions')!.getBoundingClientRect().width).toBe(32)
      );
      const bounds = rail.getBoundingClientRect();
      const frame = rail.closest('.preview-action-frame')!;
      const parent = frame.getBoundingClientRect();
      const content = frame.querySelector('.preview-frame-body')!.getBoundingClientRect();
      await expect(content.right).toBeLessThanOrEqual(bounds.left);
      await expect(getComputedStyle(rail).position).toBe('static');
      await expect(parent.right - bounds.right).toBeLessThanOrEqual(6);
      await expect(bounds.width).toBeLessThan(40);
      for (const [index, button] of buttons.entries()) {
        const rect = button.getBoundingClientRect();
        await expect(rect.bottom).toBeLessThanOrEqual(parent.bottom);
        if (index > 0) {
          await expect(rect.top).toBeGreaterThanOrEqual(
            buttons[index - 1].getBoundingClientRect().bottom
          );
        }
      }
      buttons[0].focus();
      await expect(buttons[0]).toHaveFocus();
      if (buttons.length > 1) {
        await userEvent.tab();
        await expect(buttons[1]).toHaveFocus();
      }
    }
  }
</script>

{#snippet previews()}
  <PreviewFrame class="w-64 max-w-full">
    <img src={coast} alt="Brighton coast" class="h-48 w-full preview-content object-cover" />
    {#snippet actions()}
      <PreviewAction label="Delete image" icon="icon-[uil--trash-alt]" />
      <PreviewAction label="Edit description" icon="icon-[uil--file-edit-alt]" />
    {/snippet}
  </PreviewFrame>
  <LinkPreviewCard
    preview={{
      url: 'https://example.test',
      title: 'Chatto Preview Next',
      siteName: 'Chatto Preview Next',
      description: 'Come join our community!',
      imageUrl: null
    }}
    showDismiss={false}
    canDelete
  />
  <SocialPostEmbed
    url="https://example.test/post"
    post={{
      provider: 'bluesky',
      url: 'https://example.test/post',
      author: { displayName: 'Coastal notes', handle: 'coast.example.test' },
      text: 'A quiet morning by the sea.',
      images: [{ url: coast, alt: 'Brighton coast' }]
    }}
    showDismiss={false}
    onDelete={() => {}}
  />
{/snippet}

<Story name="Preview family" asChild play={checkRails}>
  <div class="flex max-w-md flex-col items-start gap-4">
    {@render previews()}
  </div>
</Story>

<Story name="Narrow RTL" asChild play={checkRails}>
  <div dir="rtl" class="flex w-64 max-w-full flex-col items-start gap-4">
    {@render previews()}
  </div>
</Story>

<Story name="Inline row" asChild>
  <PreviewToolbar layout="row">
    <PreviewAction label="Delete file" icon="icon-[uil--trash-alt]" />
    <PreviewAction label="Edit description" icon="icon-[uil--file-edit-alt]" />
  </PreviewToolbar>
</Story>

<Story name="Frame rim" asChild play={checkRails}>
  <PreviewFrame class="w-64 max-w-full">
    <img src={coast} alt="Brighton coast" class="h-48 w-full preview-content object-cover" />
    {#snippet actions()}
      <PreviewAction label="Pause animation" icon="icon-[uil--pause]" aria-pressed={false} />
    {/snippet}
  </PreviewFrame>
</Story>

<Story name="Narrow three-action frame" asChild play={checkRails}>
  <PreviewFrame class="h-40 w-20" contentClass="preview-content">
    <img src={coast} alt="Brighton coast" class="h-full w-full preview-content object-cover" />
    {#snippet actions()}
      <PreviewAction label="Delete image" icon="icon-[uil--trash-alt]" />
      <PreviewAction label="View image" icon="icon-[uil--expand-alt]" />
      <PreviewAction label="Edit description" icon="icon-[uil--file-edit-alt]" />
    {/snippet}
  </PreviewFrame>
</Story>

<Story
  name="SVG playback"
  asChild
  play={async ({ canvas }) => {
    const pause = canvas.getByRole('button', { name: 'Pause animation' });
    await waitFor(() => expect(pause).toBeEnabled());
    const initiallyPaused = pause.getAttribute('aria-pressed') === 'true';
    pause.focus();
    await userEvent.click(pause);
    await waitFor(() => expect(pause).toHaveAttribute('aria-pressed', String(!initiallyPaused)));
    await userEvent.click(pause);
    await waitFor(() => expect(pause).toHaveAttribute('aria-pressed', String(initiallyPaused)));
  }}
>
  <div class="h-48 w-64 max-w-full">
    <SvgAnimation
      source={animationSource}
      alt="A circle moving from side to side"
      width={256}
      height={192}
    />
  </div>
</Story>
