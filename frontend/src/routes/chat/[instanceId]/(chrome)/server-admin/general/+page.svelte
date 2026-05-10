<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useMutation } from '$lib/hooks';
  import InstanceSettings from '$lib/InstanceSettings.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { Panel } from '$lib/components/admin';

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
        toast.success('Configuration reset to defaults');
      },
      onError: (err) => toast.error(err)
    }
  );
</script>

<PageTitle title="General | Server Admin" />

<PaneHeader title="General" subtitle="Server identity, branding, and messages" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <InstanceSettings />

  <Panel title="Reset" icon="iconify uil--redo">
    <p class="mb-4 text-sm text-muted">
      Restore the server name, description, MOTD, welcome message, and blocked usernames to their
      defaults. Logo and banner are not affected.
    </p>
    <Button
      variant="ghost"
      onclick={() => resetMutation.execute({})}
      loading={resetMutation.loading}
      loadingText="Resetting..."
    >
      <span class="iconify uil--redo"></span>
      Reset to Defaults
    </Button>
  </Panel>
</div>
