<!--
@component

Renders a compact, provider-neutral social-post snapshot with Chatto's native
preview-card styling. Its parent owns shared link-preview actions.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { renderedLinkTarget, handleRenderedLinkClick } from '$lib/navigation/renderedLink';
  import type { SocialPostPreviewView } from '@chatto/client/timeline/linkPreviews';

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

  /** Keep social images at a measured height before their bytes arrive. */
  function imageAspectRatio(image: SocialPostPreviewView['images'][number]): string {
    return image.width && image.height ? `${image.width} / ${image.height}` : '16 / 9';
  }

  function showImage(event: Event): void {
    (event.currentTarget as HTMLImageElement).style.visibility = '';
  }

  function hideImage(event: Event): void {
    (event.currentTarget as HTMLImageElement).style.visibility = 'hidden';
  }
</script>

<!-- Context menu only. The same actions stay available from the message action menu. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="group/preview relative my-1.5 w-full max-w-md preview-frame"
  data-testid="social-post-embed"
  data-provider={post.provider}
  oncontextmenu={onContextMenu}
>
  <div class="flex flex-col gap-3 preview-content p-3">
    <a
      {...renderedLinkTarget(url)}
      onclick={(event) => handleRenderedLinkClick(event, url)}
      onauxclick={(event) => handleRenderedLinkClick(event, url)}
      class="flex min-w-0 items-center gap-2.5"
    >
      {#if post.author?.avatarUrl}
        <img
          src={post.author.avatarUrl}
          alt=""
          class="h-10 w-10 shrink-0 rounded-full object-cover"
          onload={showImage}
          onerror={hideImage}
        />
      {:else}
        <div
          class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-surface-emphasized"
        >
          {#if post.provider === 'bluesky'}
            <span class="iconify icon-[logos--bluesky] text-xl" aria-hidden="true"></span>
          {:else}
            <span class="iconify icon-[uil--comment-alt-lines] text-xl" aria-hidden="true"></span>
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
        <span class="iconify icon-[logos--bluesky] shrink-0 text-xl" aria-hidden="true"></span>
      {:else}
        <span class="shrink-0 text-xs text-muted">{providerName}</span>
      {/if}
    </a>

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
          {contentConcealed ? m('preview.show_content') : m('preview.hide_content')}
        </button>
      </div>
    {/if}

    {#if !contentConcealed}
      {#if post.text}
        <p class="line-clamp-6 text-sm leading-relaxed whitespace-pre-wrap text-text">
          {post.text}
        </p>
      {/if}

      {#if post.images.length}
        <div class={['grid gap-1 preview-thumbnail', post.images.length > 1 ? 'grid-cols-2' : '']}>
          {#each post.images as image (image.url)}
            <img
              src={image.url}
              alt={image.alt || ''}
              class="block max-h-72 w-full object-cover"
              style:aspect-ratio={imageAspectRatio(image)}
              onload={showImage}
              onerror={hideImage}
            />
          {/each}
        </div>
      {/if}

      {#if post.externalLink && (post.externalLink.title || post.externalLink.description || post.externalLink.imageUrl)}
        {@const externalURL = post.externalLink.url}
        <a
          {...renderedLinkTarget(post.externalLink.url)}
          class="flex min-w-0 gap-3 preview-thumbnail p-2 transition-[background-color] feedback-quick hover:bg-surface-emphasized"
          onclick={(event) => {
            event.stopPropagation();
            handleRenderedLinkClick(event, externalURL);
          }}
          onauxclick={(event) => handleRenderedLinkClick(event, externalURL)}
        >
          {#if post.externalLink.imageUrl}
            <img
              src={post.externalLink.imageUrl}
              alt=""
              class="h-20 w-28 shrink-0 preview-thumbnail object-cover"
              onload={showImage}
              onerror={hideImage}
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
      {/if}

      {#if post.quotedPost && post.quotedPost.url}
        {@const quotedURL = post.quotedPost.url}
        <div
          class="flex min-w-0 flex-col gap-2 overflow-hidden surface-box p-2.5"
          data-testid="quoted-social-post"
        >
          <a
            {...renderedLinkTarget(post.quotedPost.url)}
            onclick={(event) => handleRenderedLinkClick(event, quotedURL)}
            onauxclick={(event) => handleRenderedLinkClick(event, quotedURL)}
            class="flex min-w-0 items-center gap-2"
          >
            {#if post.quotedPost.author?.avatarUrl}
              <img
                src={post.quotedPost.author.avatarUrl}
                alt=""
                class="h-7 w-7 shrink-0 rounded-full object-cover"
                onload={showImage}
                onerror={hideImage}
              />
            {:else}
              <div class="h-7 w-7 shrink-0 rounded-full bg-surface-strong"></div>
            {/if}
            <div class="flex min-w-0 items-baseline gap-1.5 text-xs">
              <span class="truncate font-medium text-text-top"
                >{displayAuthor(post.quotedPost)}</span
              >
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
                {quotedContentConcealed ? m('preview.show_content') : m('preview.hide_content')}
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
                  'grid gap-1 preview-thumbnail',
                  post.quotedPost.images.length > 1 ? 'grid-cols-2' : ''
                ]}
              >
                {#each post.quotedPost.images as image (image.url)}
                  <a
                    {...renderedLinkTarget(post.quotedPost.url)}
                    onclick={(event) => handleRenderedLinkClick(event, quotedURL)}
                    onauxclick={(event) => handleRenderedLinkClick(event, quotedURL)}
                  >
                    <img
                      src={image.url}
                      alt={image.alt || ''}
                      class="block max-h-60 w-full object-cover"
                      style:aspect-ratio={imageAspectRatio(image)}
                      onload={showImage}
                      onerror={hideImage}
                    />
                  </a>
                {/each}
              </div>
            {/if}
            {#if post.quotedPost.externalLink && (post.quotedPost.externalLink.title || post.quotedPost.externalLink.description || post.quotedPost.externalLink.imageUrl)}
              {@const externalURL = post.quotedPost.externalLink.url}
              <a
                {...renderedLinkTarget(post.quotedPost.externalLink.url)}
                onclick={(event) => handleRenderedLinkClick(event, externalURL)}
                onauxclick={(event) => handleRenderedLinkClick(event, externalURL)}
                class="flex min-w-0 gap-2 preview-thumbnail p-2"
              >
                {#if post.quotedPost.externalLink.imageUrl}
                  <img
                    src={post.quotedPost.externalLink.imageUrl}
                    alt=""
                    class="h-14 w-20 shrink-0 preview-thumbnail object-cover"
                    onload={showImage}
                    onerror={hideImage}
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
        </div>
      {/if}
    {/if}
  </div>
  {#if showDismiss && onDismiss}
    <button
      type="button"
      onclick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onDismiss?.();
      }}
      class="embed-control-button"
      aria-label={m('preview.dismiss')}
    >
      <span aria-hidden="true" class="iconify icon-[uil--times] text-sm"></span>
    </button>
  {:else if onDelete}
    <button
      type="button"
      onclick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onDelete();
      }}
      class="embed-control-button"
      aria-label={m('preview.delete')}
    >
      <span aria-hidden="true" class="iconify icon-[uil--times] text-sm"></span>
    </button>
  {/if}
</div>
