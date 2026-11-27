<!-- @component Owns the Shield Mode dialog and deadline for one server session. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { provideShieldMode } from '$lib/state/server/shieldMode';
  import { m } from '$lib/i18n/messages';
  import Deadline from '$lib/lifecycle/Deadline.svelte';
  import { FormDialog } from '$lib/ui';
  import { Checkbox } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  let { children }: { children: Snippet } = $props();
  const scope = useServerScope();
  const activeStore = scope.store;
  let privilegedModeDialogVisible = $state(false);
  let privilegedModeLoading = $state(false);
  const privilegedMode = $derived(activeStore.projection.viewer?.privilegedMode);
  let selectedRoles = $state<string[]>([]);
  const availableRoles = $derived(privilegedMode?.roleSelection?.availableRoles ?? []);
  const selectedAvailableRoles = $derived(
    availableRoles.filter((role) => selectedRoles.includes(role))
  );

  let afterActivation: (() => void) | undefined;

  function openShieldDialog(onActivated?: () => void) {
    if (privilegedModeLoading || privilegedMode?.active || !privilegedMode?.available) return;
    afterActivation = onActivated;
    selectedRoles = [];
    if (availableRoles.length) void activeStore.mentionRoles.load();
    privilegedModeDialogVisible = true;
  }

  const privilegedModeDeadline = $derived(
    privilegedMode?.active ? (privilegedMode.expiresAt?.toDate().getTime() ?? null) : null
  );

  async function setPrivilegedMode(active: boolean): Promise<void> {
    if (privilegedModeLoading) return;
    privilegedModeLoading = true;
    try {
      if (active) {
        if (privilegedMode?.roleSelection && !selectedAvailableRoles.length) return;
        await activeStore.setPrivilegedMode(
          true,
          privilegedMode?.roleSelection ? selectedAvailableRoles : undefined
        );
      } else {
        await activeStore.setPrivilegedMode(false);
      }
      if (active && privilegedModeDialogVisible && scope.isCurrent()) afterActivation?.();
      afterActivation = undefined;
      privilegedModeDialogVisible = false;
    } catch {
      toast.error(m('common.error.generic'));
    } finally {
      privilegedModeLoading = false;
    }
  }
  provideShieldMode({
    get loading() {
      return privilegedModeLoading;
    },
    open: openShieldDialog,
    deactivate: () => setPrivilegedMode(false)
  });
</script>

{@render children()}

{#if privilegedModeDeadline !== null}
  <Deadline at={privilegedModeDeadline} onreached={() => void activeStore.expirePrivilegedMode()} />
{/if}

<FormDialog
  bind:visible={privilegedModeDialogVisible}
  title={m('chat.privileged_mode.enable')}
  size="sm"
  submitTone="warning"
  disabled={!privilegedMode?.available ||
    (!!privilegedMode?.roleSelection && !selectedAvailableRoles.length)}
  submitLabel={m('chat.privileged_mode.enable')}
  submitIcon="iconify icon-[uil--shield-check]"
  loading={privilegedModeLoading}
  onsubmit={() => void setPrivilegedMode(true)}
  oncloserequest={() => (afterActivation = undefined)}
  onclose={() => (privilegedModeDialogVisible = false)}
>
  {#if availableRoles.length}
    <fieldset class="flex flex-col gap-2">
      <legend class="mb-3 text-muted">{m('chat.privileged_mode.select_roles')}</legend>
      {#each availableRoles as role (role)}
        {@const roleLabel = activeStore.mentionRoles.roles.find(
          ({ name }) => name === role
        )?.displayName}
        <Checkbox
          id={`shield-role-${role}`}
          checked={selectedRoles.includes(role)}
          disabled={privilegedModeLoading}
          onCheckedChange={(checked) => {
            selectedRoles = checked
              ? [...selectedRoles, role]
              : selectedRoles.filter((name) => name !== role);
          }}
        >
          <bdi>{roleLabel || role}</bdi>
          {#if roleLabel && roleLabel !== role}
            <bdi class="ms-1 text-muted">{role}</bdi>
          {/if}
        </Checkbox>
      {/each}
    </fieldset>
  {:else}
    {m('chat.privileged_mode.confirmation')}
  {/if}
</FormDialog>
