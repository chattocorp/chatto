<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation } from '$lib/hooks';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, TextArea, Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import FormSection from '$lib/ui/FormSection.svelte';

  let motd = $state('');
  let welcomeMessage = $state('');
  let blockedUsernames = $state('');
  let isConfigured = $state(false);

  const defaultBlockedUsernames = 'root\nadmin\nsuperuser\nop\noperator\nsupport';

  function applyConfig(cfg: {
    isConfigured: boolean;
    motd?: string | null;
    welcomeMessage?: string | null;
    blockedUsernames?: string | null;
  }) {
    motd = cfg.motd ?? '';
    welcomeMessage = cfg.welcomeMessage ?? '';
    blockedUsernames = cfg.blockedUsernames ?? defaultBlockedUsernames;
    isConfigured = cfg.isConfigured;
  }

  // Load config. Instance name + description live on /general; this page owns
  // the runtime knobs (welcome screen, MOTD, blocked usernames).
  const configQuery = useQuery(
    graphql(`
      query AdminInstanceConfig {
        admin {
          instanceConfig {
            isConfigured
            instanceName
            motd
            welcomeMessage
            blockedUsernames
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

  // Save config mutation
  const saveMutation = useMutation(
    graphql(`
      mutation UpdateInstanceConfig($input: UpdateInstanceConfigInput!) {
        admin {
          updateInstanceConfig(input: $input) {
            isConfigured
            instanceName
            motd
            welcomeMessage
            blockedUsernames
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

  // Reset config mutation
  const resetMutation = useMutation(
    graphql(`
      mutation ResetInstanceConfig {
        admin {
          resetInstanceConfig
        }
      }
    `),
    {
      onCompleted: () => {
        motd = '';
        welcomeMessage = '';
        blockedUsernames = defaultBlockedUsernames;
        isConfigured = false;
        toast.success('Configuration reset to defaults');
      },
      onError: (err) => toast.error(err)
    }
  );

  const saving = $derived(saveMutation.loading || resetMutation.loading);

  async function saveConfig() {
    await saveMutation.execute({
      input: { motd, welcomeMessage, blockedUsernames }
    });
  }
</script>

<PageTitle title="Runtime | Admin" />

<PaneHeader title="Runtime" subtitle="Welcome message, MOTD, and blocked usernames" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if configQuery.loading}
    <div class="text-muted">Loading...</div>
  {:else}
    <form
      onsubmit={(e) => {
        e.preventDefault();
        saveConfig();
      }}
      class="flex max-w-xl flex-col gap-6"
    >
      <FormSection title="Messages">
        <div class="flex flex-col gap-4">
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
        </div>
      </FormSection>

      <FormSection title="Security" bordered>
        <div class="flex flex-col gap-4">
          <TextArea
            label="Blocked Usernames"
            id="blocked-usernames"
            bind:value={blockedUsernames}
            rows={6}
            disabled={saving}
            description="One per line. Users cannot register with these names."
          />
        </div>
      </FormSection>

      <div class="flex items-center gap-4 border-t border-border pt-6">
        <Button type="submit" disabled={saving} loading={saving}>
          <span class="iconify uil--check"></span>
          Save
        </Button>

        {#if isConfigured}
          <Button
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
</div>
