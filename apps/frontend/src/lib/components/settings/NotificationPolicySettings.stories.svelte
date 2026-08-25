<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import NotificationPolicySettings from './NotificationPolicySettings.svelte';
  import NotificationPolicySettingsStoryHarness from './NotificationPolicySettingsStoryHarness.svelte';

  const componentDescription = `
  Full notification-preferences matrix. Rows are activity types. Columns are
  the server, visible room groups with their member rooms, and member direct
  messages. Product defaults initialise unconfigured server cells. Only nested
  scopes can inherit.
  `.trim();

  const { Story } = defineMeta({
    title: 'Settings/Notification policy matrix',
    component: NotificationPolicySettings,
    tags: ['autodocs'],
    parameters: {
      docs: { description: { component: componentDescription } }
    }
  });
</script>

<Story
  name="Server, groups, and rooms"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The example combines product defaults, server and group overrides, inherited room values, a room override, and a direct-message override. Cells are interactive.'
      }
    }
  }}
>
  <div class="max-w-5xl">
    <NotificationPolicySettingsStoryHarness />
  </div>
</Story>

<Story
  name="Narrow viewport"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The Activity heading remains at the start edge while all scope columns use native horizontal scrolling.'
      }
    }
  }}
>
  <div class="w-80">
    <NotificationPolicySettingsStoryHarness />
  </div>
</Story>

<Story
  name="Dark theme"
  asChild
  globals={{ theme: 'dark' }}
  parameters={{
    docs: {
      description: {
        story: 'The complete matrix and its mode legend in the dark theme.'
      }
    }
  }}
>
  <div class="max-w-5xl">
    <NotificationPolicySettingsStoryHarness />
  </div>
</Story>

<Story
  name="Load error"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'A failed policy load keeps the matrix structure visible and adds a localized error notice.'
      }
    }
  }}
>
  <div class="max-w-5xl">
    <NotificationPolicySettingsStoryHarness loadFailure />
  </div>
</Story>
