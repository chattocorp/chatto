<!-- @component Shared visual treatment for local identities, fetched pages, and preview loading/failure states. -->
<script module lang="ts">
  /** Locally available identity data; this component never fetches it. */
  export interface HoverIdentity {
    name: string;
    description?: string | null;
    image?: string | null;
    server?: string;
    icon?: string;
    kind?: 'server' | 'room' | 'user';
  }
</script>

<script lang="ts">
  import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
  import { parseWebLink } from '$lib/navigation/webLink';
  import { m } from '$lib/i18n/messages';

  let {
    href,
    local,
    preview,
    loading = false
  }: {
    href: string;
    local?: HoverIdentity;
    preview?: LinkPreviewView | null;
    loading?: boolean;
  } = $props();
  const url = $derived(parseWebLink(href));
  const host = $derived(url?.host ?? href);
  const path = $derived(url && url.pathname !== '/' ? url.pathname : '');
  const concealed = $derived(
    Boolean(preview?.socialPost?.contentWarning || preview?.socialPost?.quotedPost?.contentWarning)
  );
  const page = $derived(concealed ? null : preview);
  let brokenImage = $state<string | null>(null);
</script>

<div class="preview-content">
  {#if local}
    <div class="flex flex-col gap-3 p-3">
      <div class="flex items-center gap-3">
        <div
          class={[
            'flex size-9 shrink-0 items-center justify-center overflow-hidden bg-surface-emphasized text-subdued ring-1 ring-border ring-inset',
            local.kind === 'user' ? 'rounded-full' : 'rounded-md'
          ]}
        >
          {#if local.image && brokenImage !== local.image}
            <img
              src={local.image}
              alt=""
              class="size-full object-cover"
              onerror={() => (brokenImage = local.image ?? null)}
            />
          {:else}
            <span
              class={['iconify text-xl', local.icon ?? 'icon-[uil--comments]']}
              aria-hidden="true"
            ></span>
          {/if}
        </div>
        <div class="min-w-0 flex-1">
          <div class="line-clamp-2 font-semibold break-words text-text">{local.name}</div>
          {#if local.server}<div class="truncate text-muted">{local.server}</div>{/if}
        </div>
      </div>
      {#if local.description}<div class="line-clamp-3 whitespace-pre-wrap text-subdued">
          {local.description}
        </div>{/if}
    </div>
  {:else if loading}
    <div class="flex flex-col gap-3 p-3" role="status">
      <div class="flex items-center gap-2 text-muted">
        <span class="iconify icon-[uil--link]" aria-hidden="true"></span>{m('common.loading')}
      </div>
      <div class="flex flex-col gap-2" aria-hidden="true">
        <div class="h-4 w-4/5 rounded bg-surface-emphasized"></div>
        <div class="h-4 w-3/5 rounded bg-surface-emphasized"></div>
        <div class="mt-1 h-3 w-full rounded bg-surface-emphasized"></div>
      </div>
    </div>
  {:else if page && (page.title || page.description || page.imageUrl)}
    {#if page.imageUrl && brokenImage !== page.imageUrl}
      <img
        src={page.imageUrl}
        alt=""
        class="h-30 w-full bg-surface-emphasized object-cover"
        onerror={() => (brokenImage = page?.imageUrl ?? null)}
      />
    {/if}
    <div class="flex flex-col gap-2 p-3">
      {#if page.siteName}<div class="truncate text-muted">{page.siteName}</div>{/if}
      <div class="line-clamp-2 font-semibold break-words text-text">{page.title || host}</div>
      {#if page.description}<div class="line-clamp-3 text-subdued">{page.description}</div>{/if}
    </div>
  {:else}
    <div class="flex items-start gap-3 p-3">
      <span class="iconify mt-0.5 icon-[uil--link] shrink-0 text-xl text-muted" aria-hidden="true"
      ></span>
      <div class="min-w-0">
        <div class="truncate font-semibold text-text">{host}</div>
        <div class="mt-1 text-subdued">{m('preview.hover_unavailable')}</div>
      </div>
    </div>
  {/if}
</div>
<div class="flex items-center gap-2 px-2 py-2 text-muted">
  <span class="iconify icon-[uil--globe] shrink-0" aria-hidden="true"></span>
  <div class="min-w-0 flex-1" dir="ltr">
    <span class="sr-only">{href}</span>
    <div class="truncate" aria-hidden="true">{host}<span class="opacity-70">{path}</span></div>
  </div>
</div>
