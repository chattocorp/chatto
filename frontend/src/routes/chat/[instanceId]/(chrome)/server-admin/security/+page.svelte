<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation } from '$lib/hooks';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { Panel } from '$lib/components/admin';

  const defaultBlockedUsernames = 'root\nadmin\nsuperuser\nop\noperator\nsupport';

  let blockedUsernames = $state('');

  const configQuery = useQuery(
    graphql(`
      query AdminSecurityConfig {
        admin {
          serverConfig {
            isConfigured
            blockedUsernames
          }
        }
      }
    `),
    () => ({}),
    {
      onCompleted: (data) => {
        if (data.admin?.serverConfig) {
          blockedUsernames = data.admin.serverConfig.blockedUsernames ?? defaultBlockedUsernames;
        }
      },
      onError: (err) => toast.error(err)
    }
  );

  const saveMutation = useMutation(
    graphql(`
      mutation UpdateSecurityConfig($input: UpdateServerConfigInput!) {
        admin {
          updateServerConfig(input: $input) {
            isConfigured
            blockedUsernames
          }
        }
      }
    `),
    {
      onCompleted: (data) => {
        if (data.admin?.updateServerConfig) {
          blockedUsernames =
            data.admin.updateServerConfig.blockedUsernames ?? defaultBlockedUsernames;
          toast.success('Settings saved');
        }
      },
      onError: (err) => toast.error(err)
    }
  );

  const saving = $derived(saveMutation.loading);

  async function save(e: Event) {
    e.preventDefault();
    await saveMutation.execute({ input: { blockedUsernames } });
  }
</script>

<PageTitle title="Security | Server Admin" />

<PaneHeader title="Security" subtitle="Sign-up restrictions and account protection" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <Panel title="Blocked Usernames" icon="iconify uil--shield-exclamation">
    {#if configQuery.loading}
      <div class="text-muted">Loading...</div>
    {:else}
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
