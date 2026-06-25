<!--
@component

A `<Pill tone="server">` displaying a server's name (truncated) with
a globe icon, plus a click-triggered card that previews the server's
branding (icon, OG image, welcome message).

The data is read from `serverRegistry` and the per-server state store,
both of which are populated when a server is registered, so no extra
network round trips are needed.

```svelte
<ServerPill serverId={conv.serverId} />
```
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { Pill } from '$lib/ui';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let {
    serverId
  }: {
    serverId: string;
  } = $props();

  const server = $derived(serverRegistry.getServer(serverId));
  const store = $derived(serverRegistry.tryGetStore(serverId));

  // Hide globally when the client is only connected to a single server — the
  // pill carries no useful information in that case, and is just visual noise.
  const visible = $derived(serverRegistry.servers.length > 1);

  const name = $derived(server?.name ?? '');
  const iconUrl = $derived(store?.serverInfo.iconUrl ?? server?.iconUrl ?? null);
  const bannerUrl = $derived(store?.serverInfo.bannerUrl ?? null);
  const description = $derived(store?.serverInfo.description ?? null);
  const welcomeMessage = $derived(store?.serverInfo.welcomeMessage ?? null);
  const motd = $derived(store?.serverInfo.motd ?? null);
  const hostname = $derived.by(() => {
    if (!server) return '';
    try {
      return new URL(server.url).hostname;
    } catch {
      return server.url;
    }
  });

  // Strip markdown punctuation so the excerpt reads cleanly in a small
  // popover. We don't render full markdown here — keeping the card light
  // and predictable. Description is plain text, but we run it through the
  // same normalizer for safety with leading whitespace.
  const blurb = $derived.by(() => {
    const src = description ?? motd ?? welcomeMessage;
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

{#if visible && open && server && anchor}
  <ContextMenu
    {anchor}
    role="dialog"
    ariaLabel="Server details for {name}"
    class="w-72"
    onclose={() => (open = false)}
  >
    <div class="menu-section overflow-hidden p-0">
      {#if bannerUrl}
        <SkeletonImg src={bannerUrl} alt="" class="aspect-[1200/630] block w-full object-cover" />
      {/if}

      <div class="flex items-start gap-3 p-3">
        {#if iconUrl}
          <img src={iconUrl} alt="" class="h-10 w-10 shrink-0 rounded-md" />
        {:else}
          <div
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-server/10 text-instance"
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
