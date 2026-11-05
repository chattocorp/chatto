<script lang="ts">
  import type { SocialPostPreviewView } from '@chatto/client/timeline/linkPreviews';
  import NativeImage from './NativeImage.svelte';
  import ActionButton from './ActionButton.svelte';
  import SocialPreview from './SocialPreview.svelte';

  let {
    post,
    serverUrl,
    active,
    quoted = false
  }: {
    post: SocialPostPreviewView;
    serverUrl: string;
    active: boolean;
    quoted?: boolean;
  } = $props();
  const contentKey = $derived(`${post.url || ''}\n${post.contentWarning || ''}`);
  let revealedKey = $state<string | null>(null);
  const revealed = $derived(revealedKey === contentKey);
</script>

<div class="social">
  <text class="author">{post.author?.displayName || post.author?.handle || post.provider}</text>
  <text class="muted">{post.provider}{post.author?.handle ? ` · ${post.author.handle}` : ''}</text>
  {#if post.contentWarning}
    <text class="warning">{post.contentWarning}</text>
    <ActionButton
      label={revealed ? 'Hide content' : 'Show content'}
      onclick={() => (revealedKey = revealed ? null : contentKey)}
      testid="reveal-social"
    />
  {/if}
  {#if !post.contentWarning || revealed}
    <text class="body" testid="social-body">{post.text}</text>
    {#each post.images as image, index (`${image.url}:${index}`)}
      <NativeImage src={image.url} {serverUrl} {active} alt={image.alt || 'Social post image'} />
    {/each}
    {#if post.externalLink}
      <text class="author">{post.externalLink.title || post.externalLink.url}</text>
      {#if post.externalLink.description}<text class="body">{post.externalLink.description}</text
        >{/if}
      {#if post.externalLink.imageUrl}
        <NativeImage
          src={post.externalLink.imageUrl}
          {serverUrl}
          {active}
          alt={post.externalLink.title || 'Link image'}
        />
      {/if}
    {/if}
    {#if post.quotedPost && !quoted}
      <SocialPreview post={post.quotedPost} {serverUrl} {active} quoted={true} />
    {/if}
  {/if}
</div>

<style>
  .social {
    display: flex;
    flex-direction: column;
    gap: var(--size-8);
    padding: var(--size-12);
    border-left-width: var(--size-2);
    border-color: var(--color-action);
  }
  .author {
    font-size: var(--text-body);
    font-weight: bold;
  }
  .muted {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
  .warning {
    font-size: var(--text-body);
    color: var(--color-error);
  }
  .body {
    font-size: var(--text-body);
    white-space: pre-wrap;
    user-select: text;
  }
</style>
