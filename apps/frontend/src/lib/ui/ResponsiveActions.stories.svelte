<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent } from 'storybook/test';
  import ResponsiveActionsStoryHarness from './ResponsiveActionsStoryHarness.svelte';

  const { Story } = defineMeta({
    title: 'UI/ResponsiveActions',
    component: ResponsiveActionsStoryHarness,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'A per-container controller for inline and overflow action snippets. Attach observe to the container, inline to its action region, and trigger to its overflow button. Supply menu dismissal and replacement focus callbacks. Action state stays with the host.'
        }
      }
    }
  });
</script>

<Story
  name="Below cutoff"
  args={{ width: '319px' }}
  play={async ({ canvas }) => {
    await userEvent.click(canvas.getByRole('button', { name: 'Document actions' }));
    await userEvent.click(canvas.getByRole('button', { name: 'Notifications' }));
    await expect(canvas.getByRole('button', { name: 'Notifications' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    await userEvent.keyboard('{Escape}');
    await expect(canvas.getByRole('button', { name: 'Document actions' })).toHaveFocus();
    await userEvent.click(canvas.getByRole('button', { name: 'Wide card' }));
    await expect(canvas.getByRole('button', { name: 'Notifications' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
  }}
/>
<Story name="At cutoff" args={{ width: '320px' }} />
<Story name="Custom cutoff" args={{ width: '23rem', breakpointRem: 24 }} />
