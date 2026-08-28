<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ServerIcon from './ServerIcon.svelte';

  const { Story } = defineMeta({
    title: 'Components/ServerIcon',
    component: ServerIcon,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Server gutter icon with selection, notification, compatibility, and sign-in states.'
        }
      }
    }
  });

  const home = { name: 'Home Server', logoUrl: null };
  const remote = { name: 'Remote Server', logoUrl: null };
  const legacy = { name: 'Legacy Server', logoUrl: null };
</script>

<Story name="Gutter states" asChild>
  <div class="inline-flex flex-col gap-2 rounded-xl border border-border bg-background p-2">
    <ServerIcon server={home} href="/chat/-" title="Home Server" selected />
    <ServerIcon
      server={remote}
      href="/chat/remote"
      title="Remote Server needs sign-in"
      dimmed
      signInRequired
    />
    <ServerIcon
      server={legacy}
      href="/chat/legacy"
      title="Legacy Server — Server version is not supported"
      dimmed
      compatibilityWarning
    />
  </div>
</Story>

<Story name="Sign-in required" asChild>
  <div class="inline-flex rounded-xl border border-border bg-background p-2">
    <ServerIcon
      server={remote}
      href="/chat/remote"
      title="Remote Server needs sign-in"
      dimmed
      signInRequired
    />
  </div>
</Story>

<Story name="Route and attention" asChild>
  <div class="inline-flex flex-col gap-2 rounded-xl border border-border bg-background p-2">
    <ServerIcon server={home} href="#home" title="Home Server" selected />
    <ServerIcon server={remote} href="#remote" title="Remote Server" indicator="unread" />
    <ServerIcon
      server={legacy}
      href="#legacy"
      title="Legacy Server"
      indicator="notification"
      notificationCount={3}
      importantNotificationCount={3}
    />
  </div>
</Story>
