<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DesktopSettings from './DesktopSettings.svelte';
  import { DesktopUpdates, type DesktopUpdateStatus } from '$lib/desktop/updates.svelte';

  const { Story } = defineMeta({ title: 'Settings/Updates', component: DesktopSettings });

  /** Static host snapshots never contact an updater or install a release. */
  function preview(status: DesktopUpdateStatus['status'] | null) {
    const updates = new DesktopUpdates();
    updates.loading = false;
    if (status)
      updates.status = {
        revision: 1,
        version: '0.5.0-beta.27',
        status,
        busy: status === 'checking' || status === 'downloading' || status === 'installing',
        progress: status === 'downloading' ? 42 : null,
        availableVersion: status === 'ready' || status === 'downloading' ? '0.5.0-beta.28' : null,
        automatic: true,
        channel: 'beta'
      };
    return updates;
  }
</script>

<Story name="Ready" args={{ updates: preview('ready') }} />
<Story name="Downloading" args={{ updates: preview('downloading') }} />
<Story name="Checking" args={{ updates: preview('checking') }} />
<Story name="Up to date" args={{ updates: preview('current') }} />
<Story name="Failed" args={{ updates: preview('error') }} />
<Story name="Unavailable" args={{ updates: preview('unavailable') }} />
<Story name="Browser" args={{ updates: preview(null) }} />
<Story name="Narrow" asChild>
  <div class="flex h-96 w-80 max-w-full">
    <DesktopSettings updates={preview('ready')} />
  </div>
</Story>
