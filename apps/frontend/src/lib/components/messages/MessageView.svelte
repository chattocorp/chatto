<!--
@component

Shared presentation for one message. Timeline and search surfaces provide
their own contextual metadata and actions while this component keeps message
identity, body rendering, and row geometry consistent.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import type { UserAvatarUserView } from '$lib/render/types';
  import type { RoomMember } from '$lib/state/room';
  import type { TimeFormatSettings } from '$lib/utils/formatTime';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import DeletedUserLabel from '$lib/components/DeletedUserLabel.svelte';
  import MessageContent from '$lib/components/MessageContent.svelte';
  import * as m from '$lib/i18n/messages';

  let {
    eventId,
    actor,
    displayName,
    missingActorIsDeleted = true,
    body = null,
    deleted = false,
    edited = false,
    compact = false,
    avatarOffset = false,
    hasFooter = false,
    class: className,
    rowClass,
    members = [],
    roleHandles = [],
    timestampSettings,
    timestampLocale,
    onMentionClick,
    onActorClick,
    onActorContextMenu,
    onActorTouchStart,
    ontouchstart,
    ontouchend,
    ontouchmove,
    ontouchcancel,
    onmousedown,
    onmouseup,
    onmouseleave,
    bodyElement = $bindable(),
    compactLeading,
    prelude,
    authorSuffix,
    headerMeta,
    afterBody,
    actions
  }: {
    eventId: string;
    actor: UserAvatarUserView | null;
    displayName: string;
    missingActorIsDeleted?: boolean;
    body?: string | null;
    deleted?: boolean;
    edited?: boolean;
    compact?: boolean;
    avatarOffset?: boolean;
    hasFooter?: boolean;
    class?: ClassValue;
    rowClass?: ClassValue;
    members?: RoomMember[];
    roleHandles?: string[];
    timestampSettings?: TimeFormatSettings;
    timestampLocale?: string;
    onMentionClick?: (userId: string, anchorRect: DOMRect) => void;
    onActorClick?: (event: MouseEvent) => void;
    onActorContextMenu?: (event: MouseEvent) => void;
    onActorTouchStart?: (event: TouchEvent) => void;
    ontouchstart?: (event: TouchEvent) => void;
    ontouchend?: (event: TouchEvent) => void;
    ontouchmove?: (event: TouchEvent) => void;
    ontouchcancel?: (event: TouchEvent) => void;
    onmousedown?: (event: MouseEvent) => void;
    onmouseup?: (event: MouseEvent) => void;
    onmouseleave?: (event: MouseEvent) => void;
    bodyElement?: HTMLElement;
    compactLeading?: Snippet;
    prelude?: Snippet;
    authorSuffix?: Snippet;
    headerMeta?: Snippet;
    afterBody?: Snippet;
    actions?: Snippet;
  } = $props();

  const actorInteractive = $derived(
    actor !== null && (!!onActorClick || !!onActorContextMenu || !!onActorTouchStart)
  );
</script>

<div class={['group relative hover:z-10', className]} role="article" data-event-id={eventId}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class={[
      'group/msg group/badges message-row',
      hasFooter && 'message-row-footer',
      compact && body ? 'items-baseline' : 'items-start',
      rowClass
    ]}
    {ontouchstart}
    {ontouchend}
    {ontouchmove}
    {ontouchcancel}
    {onmousedown}
    {onmouseup}
    {onmouseleave}
  >
    {#if compact}
      <div class="flex w-11 shrink-0 items-center justify-center">
        {@render compactLeading?.()}
      </div>
    {:else}
      <div class="w-11 shrink-0"></div>
      {#if actor && !actor.deleted}
        {#if actorInteractive}
          <button
            type="button"
            class={['absolute left-2 z-10 cursor-pointer', avatarOffset ? 'top-8' : 'top-1']}
            onclick={onActorClick}
            ontouchstart={onActorTouchStart}
            oncontextmenu={onActorContextMenu}
          >
            <UserAvatar user={actor} size="message" class="shadow-md" />
          </button>
        {:else}
          <div class={['absolute left-2 z-10', avatarOffset ? 'top-8' : 'top-1']}>
            <UserAvatar user={actor} size="message" class="shadow-md" />
          </div>
        {/if}
      {:else}
        {@const deletedActor = actor?.deleted || missingActorIsDeleted}
        <div
          class={[
            'absolute left-2 z-10 flex h-11 w-11 items-center justify-center rounded-full bg-surface-emphasized text-muted shadow-md ring-1 ring-surface-emphasized/30',
            avatarOffset ? 'top-8' : 'top-1'
          ]}
          role="img"
          aria-label={deletedActor ? m['common.deleted_user']() : displayName}
        >
          <span
            class={['iconify text-xl', deletedActor ? 'uil--user-times' : 'uil--user']}
            aria-hidden="true"
          ></span>
        </div>
      {/if}
    {/if}

    <div class="message-content-stack">
      {@render prelude?.()}

      {#if !compact}
        <div class="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
          {#if actor && !actor.deleted}
            {#if actorInteractive}
              <button
                type="button"
                class="inline-flex shrink-0 cursor-pointer items-center gap-1.5 leading-none font-semibold hover:underline"
                onclick={onActorClick}
                ontouchstart={onActorTouchStart}
                oncontextmenu={onActorContextMenu}
              >
                <span>{displayName}</span>
                {@render authorSuffix?.()}
              </button>
            {:else}
              <strong class="inline-flex shrink-0 items-center gap-1.5 leading-none font-semibold">
                <span>{displayName}</span>
                {@render authorSuffix?.()}
              </strong>
            {/if}
          {:else if actor?.deleted || missingActorIsDeleted}
            <strong class="shrink-0 leading-none font-semibold text-muted">
              <DeletedUserLabel />
            </strong>
          {:else}
            <strong class="shrink-0 leading-none font-semibold text-muted">{displayName}</strong>
          {/if}
          {@render headerMeta?.()}
        </div>
      {/if}

      {#if deleted}
        <span class="text-muted italic">{m['room.message.meta.deleted']()}</span>
      {:else if body}
        <div bind:this={bodyElement} class="pointer-fine:select-text">
          <MessageContent
            {body}
            {members}
            {roleHandles}
            {edited}
            {timestampSettings}
            {timestampLocale}
            {onMentionClick}
          />
        </div>
      {/if}

      {@render afterBody?.()}
    </div>

    {@render actions?.()}
  </div>
</div>
