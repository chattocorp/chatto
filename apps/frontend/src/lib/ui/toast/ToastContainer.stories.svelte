<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';
  import ToastContainer from './ToastContainer.svelte';
  import { toast } from './toastState.svelte';

  const componentDescription = `
    The application shell mounts one toast container. It stacks toasts at the inline end of the
    viewport on wide screens and across the full width on narrow screens, announces them politely,
    and animates each entrance unless the user requests reduced motion.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/Toast Container',
    component: ToastContainer,
    tags: ['autodocs'],
    parameters: {
      layout: 'fullscreen',
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  import { untrack } from 'svelte';

  /**
   * Seeds persistent toasts and removes them when the story unmounts. Adding
   * a toast reads the toast list, so untrack keeps the attachment from
   * re-running on its own writes.
   */
  function seedToasts() {
    untrack(() => {
      toast.success('Profile settings saved', 0);
      toast.warning('Some changes could not be applied', 0);
      toast.info('A new version is available', 0, { label: 'Reload', onClick: () => {} });
    });
    return () => untrack(() => toast.clear());
  }

  let count = 0;
  function addToast() {
    count += 1;
    toast.info(`Notification ${count}`);
  }
</script>

<Story name="Stacked toasts" asChild>
  <div class="min-h-96 bg-background p-6" {@attach seedToasts}>
    <Button variant="secondary" onclick={addToast}>Show another toast</Button>
    <ToastContainer />
  </div>
</Story>
