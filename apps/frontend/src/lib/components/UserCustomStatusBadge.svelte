<!--
@component

Displays a user's custom status emoji with optional text. The status is
independent of presence and hides itself after its expiry timestamp.

**Props:**
- `status` - The custom user status to display.
- `showText` - Whether to show the status text next to the emoji.
-->
<script lang="ts">
  import type { CustomUserStatus } from '$lib/state/userProfiles.svelte';
  import { formatCustomStatusText } from '$lib/customStatusTemplates';

  const MAX_TIMEOUT_DELAY_MS = 2_147_483_647;

  let {
    status,
    showText = false,
    class: className = ''
  }: {
    status?: CustomUserStatus | null;
    showText?: boolean;
    class?: string;
  } = $props();

  let expiryTick = $state(0);

  const activeStatus = $derived.by(() => {
    void expiryTick;
    if (!status) return null;
    if (!status.expiresAt) return status;
    return new Date(status.expiresAt).getTime() > Date.now() ? status : null;
  });
  const displayText = $derived(activeStatus?.text ? formatCustomStatusText(activeStatus.text) : '');
  const title = $derived(
    activeStatus ? `${activeStatus.emoji}${displayText ? ` ${displayText}` : ''}` : undefined
  );

  $effect(() => {
    const expiresAt = status?.expiresAt;
    if (!expiresAt) return;
    let timeout: ReturnType<typeof setTimeout> | undefined;

    const schedule = () => {
      const expiresAtMs = new Date(expiresAt).getTime();
      if (Number.isNaN(expiresAtMs)) return;
      const delay = expiresAtMs - Date.now();
      if (delay <= 0) {
        expiryTick += 1;
        return;
      }
      timeout = setTimeout(schedule, Math.min(delay, MAX_TIMEOUT_DELAY_MS));
    };

    schedule();
    return () => {
      if (timeout) clearTimeout(timeout);
    };
  });
</script>

{#if activeStatus}
  <span
    class={[
      'inline-flex min-w-0 shrink-0 items-center align-middle leading-none',
      showText ? 'gap-1 text-xs text-muted' : 'text-sm',
      className
    ]}
    {title}
    aria-label={title}
  >
    <span aria-hidden="true">{activeStatus.emoji}</span>
    {#if showText && displayText}
      <span class="min-w-0 truncate">{displayText}</span>
    {/if}
  </span>
{/if}
