<!--
@component

Shared identity row for members, the current user, and call participants.
Callers own user data, presence, menus, and media. Avatar content must be
non-interactive when identityAttributes supplies a whole-identity button.
Actions are siblings of that button, so controls never nest inside a button.
-->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { formatAccountName, type AccountNameIdentity } from '$lib/render/accountName';
  import type { Snippet } from 'svelte';
  import type { ClassValue, HTMLButtonAttributes, HTMLAttributes } from 'svelte/elements';
  import CompactActionButton from './CompactActionButton.svelte';
  import VoiceActivity from './VoiceActivity.svelte';

  let {
    name,
    identity,
    username,
    avatar,
    secondary,
    badges,
    indicators,
    actions,
    voiceLevel,
    menu,
    children,
    variant = 'plain',
    identityAttributes,
    class: className,
    nameClass,
    testId,
    textTestId,
    secondaryTestId
  }: {
    name: string | Snippet;
    /** Account metadata for a string name; snippets render their own identity. */
    identity?: AccountNameIdentity | null;
    /** Canonical login, rendered consistently below the display name. */
    username?: string;
    avatar: Snippet;
    secondary?: Snippet;
    badges?: Snippet;
    indicators?: Snippet;
    actions?: Snippet;
    /** Optional normalized audio-level getter. The layer stays behind the identity header. */
    voiceLevel?: () => number;
    /** Optional overflow action. Icons and other controls go in actions. */
    menu?: {
      label: string;
      onclick: NonNullable<HTMLButtonAttributes['onclick']>;
      expanded?: boolean;
      testId?: string;
      disabled?: boolean;
      /** Reveal on hover or keyboard focus; touch devices keep the control visible. */
      revealOnHover?: boolean;
      oncontextmenu?: NonNullable<HTMLAttributes<HTMLDivElement>['oncontextmenu']>;
    };
    children?: Snippet;
    /** Row has sidebar feedback; card uses the bottom-row shell surface. */
    variant?: 'row' | 'card' | 'plain';
    /** Omit for passive identity text or an independently clickable avatar. */
    identityAttributes?: HTMLButtonAttributes;
    class?: ClassValue;
    nameClass?: ClassValue;
    testId?: string;
    textTestId?: string;
    secondaryTestId?: string;
  } = $props();
</script>

{#snippet identityContent()}
  <span class="flex shrink-0 items-center">{@render avatar()}</span>
  <span class="flex min-w-0 flex-1 flex-col overflow-hidden leading-tight" data-testid={textTestId}>
    <span class="flex min-w-0 items-center gap-1.5">
      <span class={['min-w-0 text-sm font-semibold', nameClass]} data-testid="user-card-name">
        {#if typeof name === 'string'}<AccountName {name} {identity} />{:else}{@render name()}{/if}
      </span>
      {@render badges?.()}
    </span>
    {#if secondary || username}
      <span class="block truncate text-start text-xs text-muted" data-testid={secondaryTestId}>
        {#if secondary}{@render secondary()}{:else}<bdi dir="ltr">@{username}</bdi>{/if}
      </span>
    {/if}
  </span>
  {@render indicators?.()}
{/snippet}

<div
  role="group"
  aria-label={typeof name === 'string' ? formatAccountName(name, identity) : undefined}
  class={[
    'group/user-card min-h-12 min-w-0 shrink-0',
    variant === 'card' && 'overflow-hidden shell-surface',
    variant === 'row' && 'rounded-xl focus-within:shell-surface hover:shell-surface',
    className
  ]}
  data-testid={testId}
  oncontextmenu={menu?.oncontextmenu && !menu.disabled
    ? (event) => {
        event.preventDefault();
        menu?.oncontextmenu?.(event);
      }
    : undefined}
>
  <div
    class="relative isolate flex h-12 min-h-12 min-w-0 shrink-0 items-center gap-2 rounded-[inherit] px-2"
  >
    {#if voiceLevel}<VoiceActivity level={voiceLevel} />{/if}
    {#if identityAttributes}
      <button
        type="button"
        {...identityAttributes}
        class={[
          'flex h-full min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md text-start text-text focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-neutral-action',
          identityAttributes.class
        ]}>{@render identityContent()}</button
      >
    {:else}
      <div class="flex min-w-0 flex-1 items-center gap-2">{@render identityContent()}</div>
    {/if}
    {@render actions?.()}
    {#if menu}
      <CompactActionButton
        wrapperClass={[
          'transition-opacity feedback-quick',
          menu.revealOnHover && !menu.expanded
            ? 'compact-input:hover-actions:opacity-0 group-hover/user-card:opacity-100 group-focus-within/user-card:opacity-100'
            : undefined
        ]}
        label={menu.label}
        data-testid={menu.testId}
        onclick={menu.onclick}
        disabled={menu.disabled}
        aria-haspopup="dialog"
        aria-expanded={menu.expanded ?? false}
      >
        <span class="iconify icon-[uil--ellipsis-v]" aria-hidden="true"></span>
      </CompactActionButton>
    {/if}
  </div>
  {@render children?.()}
</div>
