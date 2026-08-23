<!--
@component

A sidebar navigation row with an optional trailing status and hover action.
The link and trailing controls are rendered as siblings so callers can provide
interactive status or action snippets without nesting controls inside a link.

On devices with hover, the action replaces the status while the row is hovered.
Keyboard focus also reveals the action. The `class` prop styles the visual row;
remaining anchor attributes are forwarded to the navigation link.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import type { ClassValue, HTMLAnchorAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLAnchorAttributes, 'children' | 'class'> {
    children: Snippet;
    /** Status rendered at the trailing edge, such as an unread dot or notification badge. */
    status?: Snippet;
    /** Action that replaces the status on hover and reveals itself on keyboard focus. */
    hoverAction?: Snippet;
    /** Optional right-click or long-press behavior for the complete row. */
    contextMenuTrigger?: Attachment<HTMLElement>;
    /** Classes applied to the visual row rather than the nested navigation link. */
    class?: ClassValue;
    /** Optional stable selector for the visual row. */
    testid?: string;
  }

  let {
    children,
    status,
    hoverAction,
    contextMenuTrigger,
    class: className,
    testid,
    ...anchorAttributes
  }: Props = $props();
</script>

<div
  class={['group/sidebar-navigation-item @container sidebar-item', className]}
  data-testid={testid}
  {@attach contextMenuTrigger}
>
  <a
    {...anchorAttributes}
    class="flex min-w-0 flex-1 items-center gap-2 self-stretch focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-action"
  >
    {@render children()}
  </a>

  {#if status || hoverAction}
    <span class="relative flex h-6 min-w-6 shrink-0 items-center justify-center">
      {#if hoverAction}
        <span
          class="peer/sidebar-hover-action pointer-events-none absolute inset-0 z-10 flex items-center justify-center opacity-0 transition-opacity focus-within:pointer-events-auto focus-within:opacity-100 hover-actions:group-hover/sidebar-navigation-item:pointer-events-auto hover-actions:group-hover/sidebar-navigation-item:opacity-100"
          data-sidebar-hover-action
        >
          {@render hoverAction()}
        </span>
      {/if}

      {#if status}
        <span
          class={[
            'flex items-center justify-center',
            hoverAction &&
              'transition-opacity peer-focus-within/sidebar-hover-action:pointer-events-none peer-focus-within/sidebar-hover-action:opacity-0 hover-actions:group-hover/sidebar-navigation-item:pointer-events-none hover-actions:group-hover/sidebar-navigation-item:opacity-0'
          ]}
          data-sidebar-status
        >
          {@render status()}
        </span>
      {/if}
    </span>
  {/if}
</div>
