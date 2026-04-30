<!--
@component

A `<Pill tone="instance">` displaying an instance's name (truncated) with
a globe icon, plus a hover card that previews the instance's branding
(icon, OG image, welcome message).

The data is read from `instanceRegistry` and the per-instance state store,
both of which are populated when an instance is registered, so no extra
network round trips are needed.

```svelte
<InstancePill instanceId={conv.instanceId} />
```
-->
<script lang="ts">
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { Pill } from '$lib/ui';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  // Small grace period so brushing past the pill on the way to the card
  // doesn't immediately dismiss the popover.
  const CLOSE_DELAY_MS = 150;

  let {
    instanceId
  }: {
    instanceId: string;
  } = $props();

  const instance = $derived(instanceRegistry.getInstance(instanceId));
  const store = $derived(instanceRegistry.tryGetStore(instanceId));

  // Hide globally when the client is only connected to a single instance — the
  // pill carries no useful information in that case, and is just visual noise.
  const visible = $derived(instanceRegistry.instances.length > 1);

  const name = $derived(instance?.name ?? '');
  const iconUrl = $derived(instance?.iconUrl ?? null);
  const ogImageUrl = $derived(store?.instance.ogImageUrl ?? null);
  const welcomeMessage = $derived(store?.instance.welcomeMessage ?? null);
  const motd = $derived(store?.instance.motd ?? null);
  const hostname = $derived.by(() => {
    if (!instance) return '';
    try {
      return new URL(instance.url).hostname;
    } catch {
      return instance.url;
    }
  });

  // Strip markdown punctuation so the excerpt reads cleanly in a small
  // popover. We don't render full markdown here — keeping the card light
  // and predictable.
  const blurb = $derived.by(() => {
    const src = motd ?? welcomeMessage;
    if (!src) return null;
    const plain = src
      .replace(/^#+\s+/gm, '')
      .replace(/[*_`>]/g, '')
      .replace(/\s+/g, ' ')
      .trim();
    return plain.length > 180 ? plain.slice(0, 180).trimEnd() + '…' : plain;
  });

  let trigger = $state<HTMLSpanElement>();
  let open = $state(false);
  let anchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let closeTimer: ReturnType<typeof setTimeout> | null = null;

  function cancelClose() {
    if (closeTimer !== null) {
      clearTimeout(closeTimer);
      closeTimer = null;
    }
  }

  function scheduleClose() {
    cancelClose();
    closeTimer = setTimeout(() => {
      open = false;
      closeTimer = null;
    }, CLOSE_DELAY_MS);
  }

  function show() {
    cancelClose();
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    anchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
    open = true;
  }
</script>

{#if visible}
  <span
    bind:this={trigger}
    class="flex min-w-0 max-w-full align-middle"
    onmouseenter={show}
    onmouseleave={scheduleClose}
    onfocusin={show}
    onfocusout={scheduleClose}
    role="presentation"
  >
    <Pill tone="subtle" class="flex min-w-0 max-w-full !px-1">
      <span class="flex min-w-0 items-center gap-1">
        <span
          class="iconify shrink-0 text-xs text-instance uil--globe"
          aria-hidden="true"
        ></span>
        <span class="truncate">{name}</span>
      </span>
    </Pill>
  </span>
{/if}

{#if visible && open && instance && anchor}
  <ContextMenu
    {anchor}
    role="tooltip"
    ariaLabel="Instance details for {name}"
    class="w-72"
    onclose={() => (open = false)}
    onmouseenter={cancelClose}
    onmouseleave={scheduleClose}
  >
    <div class="menu-section overflow-hidden p-0">
      {#if ogImageUrl}
        <SkeletonImg src={ogImageUrl} alt="" class="block h-32 w-full object-cover" />
      {/if}

      <div class="flex items-start gap-3 p-3">
        {#if iconUrl}
          <img src={iconUrl} alt="" class="h-10 w-10 shrink-0 rounded-md" />
        {:else}
          <div
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-instance/10 text-instance"
          >
            <span class="iconify text-xl uil--globe" aria-hidden="true"></span>
          </div>
        {/if}
        <div class="min-w-0 flex-1">
          <div class="truncate font-semibold text-text">{name}</div>
          <div class="truncate text-xs text-muted">{hostname}</div>
        </div>
      </div>

      {#if blurb}
        <div class="border-t border-border/60 px-3 py-2 text-xs text-muted">
          {blurb}
        </div>
      {/if}
    </div>
  </ContextMenu>
{/if}
