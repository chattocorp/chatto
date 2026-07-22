<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Hint from './Hint.svelte';

  const componentDescription = `
  Inline notice box for contextual page and form messages. Use it for durable
  information, warnings, success confirmations, and recoverable errors that
  should remain in the page flow. Use \`FormError\` for field/form validation
  errors and toast notifications for transient feedback.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/Hint',
    component: Hint,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component: componentDescription
        }
      }
    }
  });
</script>

<Story
  name="Variants"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The four tones cover informational copy, successful durable state, cautionary context, and blocking page-level problems.'
      }
    }
  }}
>
  <div class="flex max-w-prose flex-col gap-3">
    <Hint tone="info">Heads up: this is informational context.</Hint>
    <Hint tone="success" icon="uil--check-circle">
      Settings saved. New members will pick this up on next login.
    </Hint>
    <Hint tone="warning" icon="uil--exclamation-triangle">
      Changes here apply immediately to all members.
    </Hint>
    <Hint tone="danger" icon="uil--exclamation-octagon">Deleting this space cannot be undone.</Hint>
  </div>
</Story>

<Story
  name="Narrow panel"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Long hint copy wraps within narrow panels instead of widening the page.'
      }
    }
  }}
>
  <div class="w-72">
    <Hint>
      Per-group overrides apply to rooms in this group. Individual rooms can add more specific
      permission overrides.
    </Hint>
  </div>
</Story>
