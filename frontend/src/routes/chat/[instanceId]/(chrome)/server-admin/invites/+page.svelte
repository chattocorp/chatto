<script lang="ts">
  import { page } from '$app/state';
  import { getActiveSpace } from '$lib/state/activeSpace.svelte';
  import { Panel } from '$lib/components/admin';
  import { Button } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const spaceId = $derived(getActiveSpace()());

  let copied = $state(false);

  function getInviteLink() {
    return `${window.location.origin}/join/${spaceId}`;
  }

  async function copyInviteLink() {
    try {
      await navigator.clipboard.writeText(getInviteLink());
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch (_e) {
      // Fallback for browsers that don't support clipboard API
      const input = document.createElement('input');
      input.value = getInviteLink();
      document.body.appendChild(input);
      input.select();
      document.execCommand('copy');
      document.body.removeChild(input);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    }
  }
</script>

<PageTitle title="Invites | Space Admin" />

<PaneHeader title="Invites" subtitle="Invite people to join this space" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <Panel title="Invite Link" icon="iconify uil--link">
    <p class="mb-4 text-sm text-muted">Share this link to invite people to join this space.</p>
    <div class="flex gap-2">
      <input type="text" readonly value={getInviteLink()} class="input flex-1 font-mono text-sm" />
      <Button variant="secondary" onclick={copyInviteLink}>
        {#if copied}
          <span class="inline-flex items-center gap-2">
            <span class="iconify uil--check"></span>
            Copied!
          </span>
        {:else}
          <span class="inline-flex items-center gap-2">
            <span class="iconify uil--copy"></span>
            Copy
          </span>
        {/if}
      </Button>
    </div>
  </Panel>
</div>
