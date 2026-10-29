<!-- @component Shared Save/Discard behavior for independent room settings panels. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Button } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';

  let {
    changed,
    disabled = false,
    onSave,
    onDiscard,
    children
  }: {
    changed: boolean;
    disabled?: boolean;
    /** Returns true after the accepted response has updated the panel's room. */
    onSave: () => Promise<boolean>;
    onDiscard: () => void;
    children: Snippet<[boolean]>;
  } = $props();

  let saving = $state(false);

  /** Pending saves also prevent navigation away from the editor. */
  export function hasChanges(): boolean {
    return changed || saving;
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (disabled || saving || !changed) return;
    saving = true;
    try {
      if (await onSave()) onDiscard();
    } finally {
      saving = false;
    }
  }
</script>

<form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
  {@render children(saving)}
  <div class="flex justify-end gap-2">
    <Button variant="secondary" disabled={saving || !changed} onclick={onDiscard}>
      {m('admin.rooms_admin.settings.discard_changes')}
    </Button>
    <Button type="submit" loading={saving} disabled={disabled || !changed}>
      {m('admin.permissions.save_changes')}
    </Button>
  </div>
</form>
