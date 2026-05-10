<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation } from '$lib/hooks';
  import InstanceSettings from '$lib/InstanceSettings.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { Panel } from '$lib/components/admin';

  let motd = $state('');
  let welcomeMessage = $state('');
  let isConfigured = $state(false);

  function applyConfig(cfg: {
    isConfigured: boolean;
    motd?: string | null;
    welcomeMessage?: string | null;
  }) {
    motd = cfg.motd ?? '';
    welcomeMessage = cfg.welcomeMessage ?? '';
    isConfigured = cfg.isConfigured;
  }

  // The admin-scoped config (motd + welcomeMessage) sits alongside the
  // public InstanceSettings form on this page. Two mutations land independently
  // (UpdateInstance for name/description, UpdateInstanceConfig for messages)
  // because the admin-scoped fields aren't on the public Mutation.updateInstance
  // input.
  const configQuery = useQuery(
    graphql(`
      query AdminGeneralMessages {
        admin {
          instanceConfig {
            isConfigured
            motd
            welcomeMessage
          }
        }
      }
    `),
    () => ({}),
    {
      onCompleted: (data) => {
        if (data.admin?.instanceConfig) {
          applyConfig(data.admin.instanceConfig);
        }
      },
      onError: (err) => toast.error(err)
    }
  );

  const saveMessagesMutation = useMutation(
    graphql(`
      mutation UpdateGeneralMessages($input: UpdateInstanceConfigInput!) {
        admin {
          updateInstanceConfig(input: $input) {
            isConfigured
            motd
            welcomeMessage
          }
        }
      }
    `),
    {
      onCompleted: (data) => {
        if (data.admin?.updateInstanceConfig) {
          applyConfig(data.admin.updateInstanceConfig);
          toast.success('Settings saved');
        }
      },
      onError: (err) => toast.error(err)
    }
  );

  const resetMutation = useMutation(
    graphql(`
      mutation ResetGeneralConfig {
        admin {
          resetInstanceConfig
        }
      }
    `),
    {
      onCompleted: () => {
        motd = '';
        welcomeMessage = '';
        isConfigured = false;
        toast.success('Configuration reset to defaults');
      },
      onError: (err) => toast.error(err)
    }
  );

  const saving = $derived(saveMessagesMutation.loading || resetMutation.loading);

  async function saveMessages(e: Event) {
    e.preventDefault();
    await saveMessagesMutation.execute({ input: { motd, welcomeMessage } });
  }
</script>

<PageTitle title="General | Server Admin" />

<PaneHeader title="General" subtitle="Server identity, branding, and messages" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <InstanceSettings />

  <Panel title="Messages" icon="iconify uil--comment-alt-message">
    {#if configQuery.loading}
      <div class="text-muted">Loading...</div>
    {:else}
      <form onsubmit={saveMessages} class="flex flex-col gap-4">
        <TextInput
          label="Message of the Day"
          id="motd"
          bind:value={motd}
          disabled={saving}
          description="Single-line message displayed in the header bar."
        />

        <TextArea
          label="Welcome Message"
          id="welcome-message"
          bind:value={welcomeMessage}
          rows={3}
          disabled={saving}
          description="Shown on the login page. Supports markdown."
        />

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={saving} loading={saving}>
            <span class="iconify uil--check"></span>
            Save
          </Button>

          {#if isConfigured}
            <Button
              type="button"
              variant="ghost"
              onclick={() => resetMutation.execute({})}
              disabled={saving}
            >
              <span class="iconify uil--redo"></span>
              Reset to Defaults
            </Button>
          {/if}
        </div>
      </form>
    {/if}
  </Panel>
</div>
