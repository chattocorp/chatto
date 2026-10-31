<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PreviewAction from './PreviewAction.svelte';

  const { Story } = defineMeta({ title: 'UI/PreviewAction', component: PreviewAction });
</script>

<Story name="Actions" asChild>
  <div class="flex gap-1">
    <PreviewAction label="Delete attachment" icon="icon-[uil--trash-alt]" />
    <PreviewAction label="Edit description" icon="icon-[uil--file-edit-alt]" />
    <PreviewAction label="View attachment" icon="icon-[uil--expand-alt]" />
    <PreviewAction label="Unavailable action" icon="icon-[uil--times]" disabled />
  </div>
</Story>
