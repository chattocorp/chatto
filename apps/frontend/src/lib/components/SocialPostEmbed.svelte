<!--
@component

Renders a compact, provider-neutral social-post snapshot with Chatto's native
preview-card styling. Its parent owns shared link-preview actions.
-->
<script lang="ts">
  import * as m from '$lib/i18n/messages';
  import type { SocialPostPreviewView } from '$lib/render/linkPreviews';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let {
    url,
    post,
    onDismiss,
    showDismiss = true,
    onContextMenu,
    onDelete
  }: {
    url: string;
    post: SocialPostPreviewView;
    onDismiss?: () => void;
    showDismiss?: boolean;
    onContextMenu?: (event: MouseEvent) => void;
    onDelete?: () => void;
  } = $props();

  const providerName = $derived(post.provider === 'bluesky' ? 'Bluesky' : post.provider);
  const authorName = $derived(post.author?.displayName || post.author?.handle || providerName);
  const authorHandle = $derived(
    post.author?.handle ? `@${post.author.handle.replace(/^@/, '')}` : ''
  );

  const contentKey = $derived(`${post.url || url}\n${post.contentWarning || ''}`);
  const quotedContentKey = $derived(
    `${post.quotedPost?.url || ''}\n${post.quotedPost?.contentWarning || ''}`
  );
  let revealedContentKey = $state<string | null>(null);
  let revealedQuotedContentKey = $state<string | null>(null);
  const contentConcealed = $derived(
    Boolean(post.contentWarning) && revealedContentKey !== contentKey
  );
  const quotedContentConcealed = $derived(
    Boolean(post.quotedPost?.contentWarning) && revealedQuotedContentKey !== quotedContentKey
  );

  function displayAuthor(post: SocialPostPreviewView) {
    return post.author?.displayName || post.author?.handle || post.provider;
  }

  function displayHandle(post: SocialPostPreviewView) {
    return post.author?.handle ? `@${post.author.handle.replace(/^@/, '')}` : '';
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="group/preview relative embed-frame flex w-full max-w-md flex-col gap-3 p-3"
  data-testid="social-post-embed"
  data-provider={post.provider}
  oncontextmenu={onContextMenu}
>
  <!-- eslint-disable svelte/no-navigation-without-resolve -- url is a third-party social-post URL -->
  <a href={url} target="_blank" rel="noopener noreferrer" class="flex min-w-0 items-center gap-2.5">
    {#if post.author?.avatarUrl}
      <SkeletonImg
        src={post.author.avatarUrl}
        alt=""
        class="h-10 w-10 shrink-0 rounded-full object-cover"
      />
    {:else}
      <div
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-surface-strong"
      >
        {#if post.provider === 'bluesky'}
          <span class="iconify text-xl logos--bluesky" aria-hidden="true"></span>
        {:else}
          <span class="iconify text-xl uil--comment-alt-lines" aria-hidden="true"></span>
        {/if}
      </div>
    {/if}
    <div class="min-w-0 flex-1">
      <div class="truncate text-sm font-medium text-text-top">{authorName}</div>
      {#if authorHandle}
        <div class="truncate text-xs text-muted">{authorHandle}</div>
      {/if}
    </div>
    {#if post.provider === 'bluesky'}
      <span class="iconify shrink-0 text-xl logos--bluesky" aria-hidden="true"></span>
    {:else}
      <span class="shrink-0 text-xs text-muted">{providerName}</span>
    {/if}
  </a>
  <!-- eslint-enable svelte/no-navigation-without-resolve -->

  {#if post.contentWarning}
    <div class="flex items-center justify-between gap-2 surface-box px-2 py-1 text-xs">
      <p class="min-w-0 font-medium text-text">{post.contentWarning}</p>
      <button
        type="button"
        class="shrink-0 cursor-pointer font-medium link"
        onclick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          revealedContentKey = contentConcealed ? contentKey : null;
        }}
      >
        {contentConcealed ? m['preview.show_content']() : m['preview.hide_content']()}
      </button>
    </div>
  {/if}

  {#if !contentConcealed}
    {#if post.text}
      <p class="line-clamp-6 text-sm leading-relaxed whitespace-pre-wrap text-text">{post.text}</p>
    {/if}

    {#if post.images.length}
      <div
        class={[
          'grid gap-1 overflow-hidden rounded-sm',
          post.images.length > 1 ? 'grid-cols-2' : ''
        ]}
      >
        {#each post.images as image (image.url)}
          <SkeletonImg src={image.url} alt={image.alt || ''} class="max-h-72 w-full object-cover" />
        {/each}
      </div>
    {/if}

    {#if post.externalLink && (post.externalLink.title || post.externalLink.description || post.externalLink.imageUrl)}
      <!-- eslint-disable svelte/no-navigation-without-resolve -- destination is a third-party URL embedded in the post -->
      <a
        href={post.externalLink.url}
        target="_blank"
        rel="noopener noreferrer"
        class="flex min-w-0 gap-3 overflow-hidden surface-box p-2 transition-[background-color] hover:bg-surface-emphasized"
        onclick={(event) => event.stopPropagation()}
      >
        {#if post.externalLink.imageUrl}
          <SkeletonImg
            src={post.externalLink.imageUrl}
            alt=""
            class="h-20 w-28 shrink-0 rounded-sm object-cover"
          />
        {/if}
        <div class="flex min-w-0 flex-1 flex-col justify-center gap-0.5">
          {#if post.externalLink.title}
            <div class="line-clamp-2 text-sm font-medium text-text-top">
              {post.externalLink.title}
            </div>
          {/if}
          {#if post.externalLink.description}
            <div class="line-clamp-2 text-xs text-muted">{post.externalLink.description}</div>
          {/if}
        </div>
      </a>
      <!-- eslint-enable svelte/no-navigation-without-resolve -->
    {/if}

    {#if post.quotedPost && post.quotedPost.url}
      <div
        class="flex min-w-0 flex-col gap-2 overflow-hidden surface-box p-2.5"
        data-testid="quoted-social-post"
      >
        <!-- eslint-disable svelte/no-navigation-without-resolve -- destination is a third-party social-post URL -->
        <a
          href={post.quotedPost.url}
          target="_blank"
          rel="noopener noreferrer"
          class="flex min-w-0 items-center gap-2"
        >
          {#if post.quotedPost.author?.avatarUrl}
            <SkeletonImg
              src={post.quotedPost.author.avatarUrl}
              alt=""
              class="h-7 w-7 shrink-0 rounded-full object-cover"
            />
          {:else}
            <div class="h-7 w-7 shrink-0 rounded-full bg-surface-strong"></div>
          {/if}
          <div class="flex min-w-0 items-baseline gap-1.5 text-xs">
            <span class="truncate font-medium text-text-top">{displayAuthor(post.quotedPost)}</span>
            {#if displayHandle(post.quotedPost)}
              <span class="truncate text-muted">{displayHandle(post.quotedPost)}</span>
            {/if}
          </div>
        </a>
        {#if post.quotedPost.contentWarning}
          <div
            class="flex items-center justify-between gap-2 rounded-sm bg-surface-strong px-2 py-1 text-xs"
          >
            <p class="min-w-0 font-medium text-text">{post.quotedPost.contentWarning}</p>
            <button
              type="button"
              class="shrink-0 cursor-pointer font-medium link"
              onclick={(event) => {
                event.preventDefault();
                event.stopPropagation();
                revealedQuotedContentKey = quotedContentConcealed ? quotedContentKey : null;
              }}
            >
              {quotedContentConcealed ? m['preview.show_content']() : m['preview.hide_content']()}
            </button>
          </div>
        {/if}
        {#if !quotedContentConcealed}
          {#if post.quotedPost.text}
            <p class="line-clamp-5 text-sm leading-relaxed whitespace-pre-wrap text-text">
              {post.quotedPost.text}
            </p>
          {/if}
          {#if post.quotedPost.images.length}
            <div
              class={[
                'grid gap-1 overflow-hidden rounded-sm',
                post.quotedPost.images.length > 1 ? 'grid-cols-2' : ''
              ]}
            >
              {#each post.quotedPost.images as image (image.url)}
                <a href={post.quotedPost.url} target="_blank" rel="noopener noreferrer">
                  <SkeletonImg
                    src={image.url}
                    alt={image.alt || ''}
                    class="max-h-60 w-full object-cover"
                  />
                </a>
              {/each}
            </div>
          {/if}
          {#if post.quotedPost.externalLink && (post.quotedPost.externalLink.title || post.quotedPost.externalLink.description || post.quotedPost.externalLink.imageUrl)}
            <a
              href={post.quotedPost.externalLink.url}
              target="_blank"
              rel="noopener noreferrer"
              class="flex min-w-0 gap-2 overflow-hidden rounded-sm bg-surface-strong p-2"
            >
              {#if post.quotedPost.externalLink.imageUrl}
                <SkeletonImg
                  src={post.quotedPost.externalLink.imageUrl}
                  alt=""
                  class="h-14 w-20 shrink-0 rounded-sm object-cover"
                />
              {/if}
              <div class="min-w-0 self-center">
                {#if post.quotedPost.externalLink.title}
                  <div class="line-clamp-1 text-xs font-medium text-text-top">
                    {post.quotedPost.externalLink.title}
                  </div>
                {/if}
                {#if post.quotedPost.externalLink.description}
                  <div class="line-clamp-2 text-xs text-muted">
                    {post.quotedPost.externalLink.description}
                  </div>
                {/if}
              </div>
            </a>
          {/if}
        {/if}
        <!-- eslint-enable svelte/no-navigation-without-resolve -->
      </div>
    {/if}
  {/if}

  {#if showDismiss && onDismiss}
    <button
      type="button"
      onclick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onDismiss?.();
      }}
      class="embed-control-button md:group-hover/preview:opacity-100"
      aria-label={m['preview.dismiss']()}
    >
      <span class="iconify text-sm uil--times"></span>
    </button>
  {:else if onDelete}
    <button
      type="button"
      onclick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onDelete();
      }}
      class="embed-control-button md:group-hover/preview:opacity-100"
      aria-label={m['preview.delete']()}
    >
      <span class="iconify text-sm uil--times"></span>
    </button>
  {/if}
</div>
