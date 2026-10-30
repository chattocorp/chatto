<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LinkHoverContent from './LinkHoverContent.svelte';
  import coast from '../../../../e2e/fixtures/brighton.jpg?url';
  const { Story } = defineMeta({
    title: 'Messages/Link preview design',
    component: LinkHoverContent
  });
  const href = 'https://journal.example.com/travel/a-weekend-by-the-sea';
  const preview = {
    url: href,
    siteName: 'The Weekend Journal',
    title: 'A slower weekend by the sea',
    description: 'Coastal walks, independent cafés, and a fresh perspective on a familiar place.',
    imageUrl: coast
  };
</script>

<Story name="Preview collection" asChild>
  <div class="flex flex-wrap items-start gap-6 bg-background p-6 text-base leading-snug text-text">
    <div class="preview-surface max-w-full">
      <LinkHoverContent {href} {preview} />
    </div>
    <div class="flex w-72 max-w-full flex-col gap-6">
      <div class="preview-surface max-w-full">
        <LinkHoverContent
          href="https://chat.example.com/chat/-/development"
          local={{
            kind: 'room',
            name: 'Development',
            server: 'Chatto community',
            icon: 'icon-[uil--comments]',
            description:
              'A place to build together. Share work in progress, discuss ideas, and help shape what comes next.'
          }}
        />
      </div>
      <div class="preview-surface max-w-full">
        <LinkHoverContent
          href="https://docs.example.com/getting-started"
          preview={{
            url: 'https://docs.example.com/getting-started',
            siteName: 'Chatto Docs',
            title: 'Make yourself at home',
            description:
              'Everything you need to set up your server and bring your community together.'
          }}
        />
      </div>
    </div>
  </div>
</Story>
<Story name="Loading and unavailable" asChild>
  <div class="flex flex-wrap items-start gap-6 bg-background p-6 text-base leading-snug text-text">
    <div class="preview-surface max-w-full">
      <LinkHoverContent {href} loading />
    </div>
    <div class="preview-surface max-w-full">
      <LinkHoverContent {href} />
    </div>
  </div>
</Story>
<Story name="Narrow preview" asChild>
  <div class="w-72 max-w-full bg-background p-2 text-base leading-snug text-text">
    <div class="preview-surface max-w-full">
      <LinkHoverContent {href} {preview} />
    </div>
  </div>
</Story>
