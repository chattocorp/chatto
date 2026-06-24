<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation } from '$lib/hooks';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { Panel } from '$lib/components/admin';
  import * as m from '$lib/i18n/messages';

  const defaultBlockedUsernames = 'root\nadmin\nsuperuser\nop\noperator\nsupport';

  let blockedUsernames = $state('');

  const configQuery = useQuery(
    graphql(`
      query AdminSecurityConfig {
        admin {
          serverConfig {
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
      mutation UpdateBlockedUsernames($input: UpdateBlockedUsernamesInput!) {
        admin {
          updateBlockedUsernames(input: $input)
        }
      }
    `),
    {
      onCompleted: (data) => {
        if (data.admin) {
          blockedUsernames = data.admin.updateBlockedUsernames;
          toast.success(m['admin.security.settings_saved']());
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

<PageTitle
  title={m['admin.common.server_admin_page_title']({ title: m['admin.security.title']() })}
/>

<PaneHeader
  title={m['admin.security.title']()}
  subtitle={m['admin.security.subtitle']()}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <Panel title={m['admin.security.blocked_usernames']()} icon="iconify uil--shield-exclamation">
    {#if configQuery.loading}
      <div class="text-muted">{m['admin.common.loading']()}</div>
    {:else}
      <form onsubmit={save} class="flex flex-col gap-4">
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
