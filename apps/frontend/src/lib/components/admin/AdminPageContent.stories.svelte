<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AdminPageContent from './AdminPageContent.svelte';

  const { Story } = defineMeta({
    title: 'Admin/AdminPageContent',
    component: AdminPageContent,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Shared page frame for Server Admin content. It supplies the standard readable width, page padding, and scrolling; pages supply their own content stack.'
        }
      }
    }
  });
</script>

<Story name="Panel stack" asChild>
  <div class="h-80 w-full bg-background">
    <AdminPageContent fillHeight>
      <div class="flex min-h-0 flex-1 flex-col">
        <div class="panel-shell flex min-h-0 flex-1 flex-col p-6">Primary panel</div>
      </div>
    </AdminPageContent>
  </div>
</Story>
