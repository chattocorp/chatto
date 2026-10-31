<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PreviewToolbar from './PreviewToolbar.svelte';
  import PreviewAction from './PreviewAction.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import SocialPostEmbed from '$lib/components/SocialPostEmbed.svelte';
  import coast from '../../../e2e/fixtures/brighton.jpg?url';

  const { Story } = defineMeta({ title: 'UI/PreviewToolbar', component: PreviewToolbar });
</script>

{#snippet previews()}
  <div class="group/preview relative w-64 max-w-full preview-frame">
    <img src={coast} alt="Brighton coast" class="h-48 w-full preview-content object-cover" />
    <PreviewToolbar>
      <PreviewAction label="Delete image" icon="icon-[uil--trash-alt]" />
      <PreviewAction label="Edit description" icon="icon-[uil--file-edit-alt]" />
    </PreviewToolbar>
  </div>
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

<Story name="Preview family" asChild>
  <div class="flex max-w-md flex-col items-start gap-4">
    {@render previews()}
  </div>
</Story>

<Story name="Narrow RTL" asChild>
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
