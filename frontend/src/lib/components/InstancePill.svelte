<!--
@component

A `<Pill tone="instance">` displaying an instance's name (truncated) with
a globe icon, plus a click-triggered card that previews the instance's
branding (icon, OG image, welcome message).

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

  let trigger = $state<HTMLButtonElement>();
  let open = $state(false);
  let anchor = $state<{ top: number; bottom: number; left: number } | null>(null);

  function toggle(event: MouseEvent) {
    // Prevent ancestors (e.g. the DM-list <a>) from also reacting to the click.
    event.stopPropagation();
    event.preventDefault();
    if (open) {
      open = false;
      return;
    }
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    anchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
    open = true;
  }
</script>

{#if visible}
  <button
    bind:this={trigger}
    type="button"
    class="flex min-w-0 max-w-full cursor-pointer bg-transparent p-0 text-left align-middle"
    onclick={toggle}
    onpointerdown={(e) => e.stopPropagation()}
    aria-haspopup="dialog"
    aria-expanded={open}
  >
    <Pill
      tone="subtle"
      class="shimmer-hover relative flex min-w-0 max-w-full overflow-hidden !px-1"
    >
      <span class="flex min-w-0 items-center gap-1">
        <span
          class="iconify shrink-0 text-xs text-instance uil--globe"
          aria-hidden="true"
        ></span>
        <span class="truncate">{name}</span>
      </span>
    </Pill>
  </button>
{/if}

{#if visible && open && instance && anchor}
  <ContextMenu
    {anchor}
    role="dialog"
    ariaLabel="Instance details for {name}"
    class="w-72"
    onclose={() => (open = false)}
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
