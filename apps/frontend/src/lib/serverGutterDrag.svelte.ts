/** Lazily loaded mouse drag behavior for the server gutter. */
import { untrack } from 'svelte';
import type { ActionReturn } from 'svelte/action';
import { dndzone, type DndEvent, type Options } from 'svelte-dnd-action';

/** A drag item retains the server ID when the library changes its shadow ID. */
export type GutterItem = { id: string; serverId: string; isDndShadowItem?: boolean };

/**
 * Mount a drag zone after its module loads. The effect root owns reactive
 * action updates and DOM listeners; the returned function disposes both.
 */
export function attachServerGutterDrag(
  node: HTMLDivElement,
  options: {
    items: () => GutterItem[];
    consider: (event: DndEvent<GutterItem>) => void;
    finalize: (event: DndEvent<GutterItem>) => void;
  }
): () => void {
  return $effect.root(() => {
    let dragging = false;
    let suppressClickUntil = 0;
    let zone: ActionReturn<Options<GutterItem>> | undefined;
    const consider = (event: Event) => {
      if (event.target !== node) return;
      dragging = true;
      options.consider((event as CustomEvent<DndEvent<GutterItem>>).detail);
    };
    const finalize = (event: Event) => {
      if (event.target !== node) return;
      const detail = (event as CustomEvent<DndEvent<GutterItem>>).detail;
      dragging = false;
      if (detail.info.source === 'pointer') suppressClickUntil = performance.now() + 250;
      options.finalize(detail);
    };
    const click = (event: MouseEvent) => {
      if (dragging || (event.detail > 0 && performance.now() < suppressClickUntil)) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    // Notification buttons and modified link clicks keep their own behavior.
    const mouseDown = (event: MouseEvent) => {
      if (
        (event.target instanceof Element && event.target.closest('button')) ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) {
        event.stopPropagation();
      }
    };
    // Keep touch input native, including on devices with an attached mouse.
    // The drag library cancels touchstart even for taps. Stop it in capture
    // without preventing clicks, scrolling, or the pointer-based long press.
    const touchStart = (event: TouchEvent) => {
      event.stopPropagation();
    };
    node.addEventListener('consider', consider);
    node.addEventListener('finalize', finalize);
    node.addEventListener('click', click, true);
    node.addEventListener('mousedown', mouseDown, true);
    node.addEventListener('touchstart', touchStart, { capture: true, passive: true });
    // dndzone maps DOM children to item indices on every update. Run after
    // Svelte renders the new order, so the next drag selects the right item.
    $effect(() => {
      const config = {
        items: options.items(),
        type: 'server-gutter',
        dropFromOthersDisabled: true,
        flipDurationMs: 0,
        dropTargetStyle: {},
        autoAriaDisabled: true,
        zoneTabIndex: -1,
        zoneItemTabIndex: -1
      };
      // Library callbacks must not add dependencies to this effect.
      untrack(() => {
        if (zone) {
          zone.update?.(config);
        } else {
          zone = dndzone(node, config);
          node.dataset.serverDragReady = 'true';
        }
      });
    });
    return () => {
      delete node.dataset.serverDragReady;
      node.removeEventListener('consider', consider);
      node.removeEventListener('finalize', finalize);
      node.removeEventListener('click', click, true);
      node.removeEventListener('mousedown', mouseDown, true);
      node.removeEventListener('touchstart', touchStart, true);
      zone?.destroy?.();
    };
  });
}
