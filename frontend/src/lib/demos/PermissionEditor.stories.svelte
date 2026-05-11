<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Demos/Permission editor'
  });
</script>

<script lang="ts">
  import Pill from '$lib/ui/Pill.svelte';
  import ToggleChip from '$lib/ui/ToggleChip.svelte';
  import HelpTooltip from '$lib/ui/HelpTooltip.svelte';
  import Hint from '$lib/ui/Hint.svelte';

  type State = 'allow' | 'deny' | 'neutral';

  type Permission = {
    id: string;
    label: string;
    help: string;
    inherited: 'allow' | 'deny' | 'neutral';
    state: State;
  };

  let permissions = $state<Permission[]>([
    {
      id: 'message.post',
      label: 'message.post',
      help: 'Send messages to the room.',
      inherited: 'allow',
      state: 'neutral'
    },
    {
      id: 'message.post-in-thread',
      label: 'message.post-in-thread',
      help: 'Reply inside an existing thread, even when the room blocks new top-level posts.',
      inherited: 'allow',
      state: 'allow'
    },
    {
      id: 'message.edit-own',
      label: 'message.edit-own',
      help: 'Edit your own messages after sending.',
      inherited: 'allow',
      state: 'neutral'
    },
    {
      id: 'message.delete-own',
      label: 'message.delete-own',
      help: 'Delete your own messages after sending.',
      inherited: 'deny',
      state: 'neutral'
    },
    {
      id: 'rooms.create',
      label: 'rooms.create',
      help: 'Create new rooms inside this space.',
      inherited: 'allow',
      state: 'deny'
    }
  ]);

  function set(perm: Permission, target: State) {
    perm.state = perm.state === target ? 'neutral' : target;
  }

  function effective(perm: Permission): 'allow' | 'deny' {
    if (perm.state === 'allow') return 'allow';
    if (perm.state === 'deny') return 'deny';
    return perm.inherited === 'deny' ? 'deny' : 'allow';
  }
</script>

<Story name="Role permissions" asChild>
  <div class="mx-auto max-w-3xl p-6">
    <header class="mb-4 flex items-baseline justify-between">
      <div>
        <h1 class="text-2xl font-bold">Role permissions</h1>
        <p class="text-sm text-muted">Member role · #general</p>
      </div>
      <Pill tone="server">Room override</Pill>
    </header>

    <Hint tone="info">
      Allow / Deny here override what's inherited from the space-level role.
      Leave both released to inherit.
    </Hint>

    <table class="mt-6 w-full">
      <thead>
        <tr class="border-b border-border text-left text-xs uppercase text-muted">
          <th class="pb-2">Permission</th>
          <th class="pb-2">Inherited</th>
          <th class="pb-2">Override</th>
          <th class="pb-2">Effective</th>
        </tr>
      </thead>
      <tbody>
        {#each permissions as perm (perm.id)}
          <tr class="border-b border-border/40">
            <td class="py-3">
              <div class="flex items-center gap-2">
                <code class="font-mono text-sm">{perm.label}</code>
                <HelpTooltip>{perm.help}</HelpTooltip>
              </div>
            </td>
            <td class="py-3">
              <Pill tone={perm.inherited === 'allow' ? 'success' : perm.inherited === 'deny' ? 'danger' : 'muted'} dimmed>
                {perm.inherited === 'neutral' ? 'inherit' : perm.inherited}
              </Pill>
            </td>
            <td class="py-3">
              <div class="flex gap-2">
                <ToggleChip
                  tone="success"
                  pressed={perm.state === 'allow'}
                  onclick={() => set(perm, 'allow')}
                >
                  Allow
                </ToggleChip>
                <ToggleChip
                  tone="danger"
                  pressed={perm.state === 'deny'}
                  onclick={() => set(perm, 'deny')}
                >
                  Deny
                </ToggleChip>
              </div>
            </td>
            <td class="py-3">
              <Pill tone={effective(perm) === 'allow' ? 'success' : 'danger'}>
                {effective(perm)}
              </Pill>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</Story>
