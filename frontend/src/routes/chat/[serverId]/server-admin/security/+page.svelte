<script lang="ts">
  import { onMount } from 'svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Hint } from '$lib/ui';
  import { TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { Panel } from '$lib/components/admin';
  import {
    GetAdminSecurityConfigRequest,
    UpdateBlockedUsernamesRequest
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';

  const defaultBlockedUsernames = 'root\nadmin\nsuperuser\nop\noperator\nsupport';

  let blockedUsernames = $state(defaultBlockedUsernames);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);

  onMount(() => {
    void loadConfig();
  });

  async function loadConfig() {
    loading = true;
    error = null;

    try {
      const response = await withActiveServerWireClient((client) =>
        client.getAdminSecurityConfig(new GetAdminSecurityConfigRequest())
      );
      blockedUsernames = response.config?.blockedUsernames ?? defaultBlockedUsernames;
    } catch (err) {
      error = 'Failed to load security settings';
      toast.error(error);
      console.error('Failed to load security settings', err);
    } finally {
      loading = false;
    }
  }

  async function save(e: Event) {
    e.preventDefault();
    if (saving) return;

    saving = true;
    error = null;

    try {
      const response = await withActiveServerWireClient((client) =>
        client.updateBlockedUsernames(new UpdateBlockedUsernamesRequest({ blockedUsernames }))
      );
      blockedUsernames = response.config?.blockedUsernames ?? blockedUsernames;
      toast.success('Settings saved');
    } catch (err) {
      error = 'Failed to save security settings';
      toast.error(error);
      console.error('Failed to save security settings', err);
    } finally {
      saving = false;
    }
  }
</script>

<PageTitle title="Security | Server Admin" />

<PaneHeader title="Security" subtitle="Sign-up restrictions and account protection" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <Panel title="Blocked Usernames" icon="iconify uil--shield-exclamation">
    {#if loading}
      <div class="text-muted">Loading...</div>
    {:else}
      {#if error}
        <Hint tone="danger" icon="uil--exclamation-octagon">{error}</Hint>
      {/if}

      <form onsubmit={save} class="flex flex-col gap-4">
        <TextArea
          label="Blocked Usernames"
          id="blocked-usernames"
          bind:value={blockedUsernames}
          rows={6}
          disabled={saving}
          description="One per line. Users cannot register with these names."
        />

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={saving} loading={saving}>
            <span class="iconify uil--check"></span>
            Save
          </Button>
        </div>
      </form>
    {/if}
  </Panel>
</div>
