<script lang="ts">
  import { onMount } from 'svelte';
  import { getServerSecurityConfig, updateBlockedUsernames } from '$lib/api-client/serverState';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { AdminPageContent, Panel } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import * as m from '$lib/i18n/messages';

  const connection = useConnection();

  let blockedUsernames = $state('');
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);

  function apiConfig() {
    const conn = connection();
    return {
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    };
  }

  async function loadSecurityConfig() {
    loading = true;
    error = null;
    try {
      const config = await getServerSecurityConfig(apiConfig());
      blockedUsernames = config.blockedUsernames;
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
      toast.error(error);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void loadSecurityConfig();
  });

  async function save(e: Event) {
    e.preventDefault();
    saving = true;
    error = null;
    try {
      const config = await updateBlockedUsernames(apiConfig(), blockedUsernames);
      blockedUsernames = config.blockedUsernames;
      toast.success(m['admin.security.settings_saved']());
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
      toast.error(error);
    } finally {
      saving = false;
    }
  }
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({ title: m['admin.security.title']() })}
/>

<PaneHeader
  title={m['admin.security.title']()}
  subtitle={m['admin.security.subtitle']()}
  showMobileNav
/>

<AdminPageContent>
  <div class="flex flex-col gap-6">
  <Panel title={m['admin.security.blocked_usernames']()} icon="iconify uil--shield-exclamation">
    {#if loading}
      <div class="text-muted">{m['admin.common.loading']()}</div>
    {:else}
      <form onsubmit={save} class="flex flex-col gap-4">
        {#if error}
          <Hint tone="danger">{error}</Hint>
        {/if}

        <TextArea
          label={m['admin.security.blocked_usernames']()}
          id="blocked-usernames"
          bind:value={blockedUsernames}
          rows={6}
          disabled={saving}
          description={m['admin.security.blocked_usernames_description']()}
        />

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={saving} loading={saving}>
            <span class="iconify uil--check"></span>
            {m['rbac.role_form.save']()}
          </Button>
        </div>
      </form>
    {/if}
  </Panel>
  </div>
</AdminPageContent>
