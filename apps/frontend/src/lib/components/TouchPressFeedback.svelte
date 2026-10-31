<!--
@component
Shows immediate feedback for primary pointer presses on semantic actions. Mount once at
the app root. Movement, cancellation, release, and lost focus clear the cue;
this component never captures pointers or prevents native input behavior. A
clipped ripple finishes after release, but cancels on movement or interruption.
Custom actions opt in with data-press-feedback. A data-press-feedback="none"
ancestor excludes drag surfaces, backdrops, and other unsuitable targets.
An action with data-press-feedback="surface" uses its nearest data-press-surface
ancestor for feedback. This keeps transparent hit areas and visible content aligned.
-->
<script lang="ts">
  import { motionPreference } from '$lib/dom/motionPreference';
  import { rowLinkForTarget } from '$lib/ui/dataTableRowLinks';

  const ACTION =
    'button, a[href], summary, [role="button"], [role="link"], [role="tab"], [role="menuitem"], [role="menuitemcheckbox"], [role="menuitemradio"], [role="option"], [role="checkbox"], [role="radio"], [role="switch"], [data-press-feedback], input:is([type="button"], [type="submit"], [type="reset"], [type="checkbox"], [type="radio"]), label:has(input:is([type="checkbox"], [type="radio"]))';
  let pressed: Element | undefined;
  let pointerId: number | undefined;
  let startX = 0;
  let startY = 0;
  let cancelRipple: (() => void) | undefined;

  function release() {
    pressed?.removeAttribute('data-touch-pressed');
    pressed?.removeAttribute('data-press-inline');
    pressed = undefined;
    pointerId = undefined;
  }

  function clear() {
    release();
    cancelRipple?.();
  }

  /** Add a decorative layer without clipping the control's existing content. */
  function ripple(action: HTMLElement, event: PointerEvent) {
    if (motionPreference().matches) return;
    const rect = action.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    // Convert viewport coordinates to the control's padding box, including scale.
    const x = (event.clientX - rect.left) * (action.offsetWidth / rect.width) - action.clientLeft;
    const y = (event.clientY - rect.top) * (action.offsetHeight / rect.height) - action.clientTop;
    // Inline links and mentions have no client box, but still have a visible
    // border box. Keep their text wrapping while sizing the ripple to that box.
    const width = action.clientWidth || action.offsetWidth;
    const height = action.clientHeight || action.offsetHeight;
    const radius = Math.hypot(Math.max(x, width - x), Math.max(y, height - y));
    // Table cells are reliable containing blocks in Safari; rows are not.
    // Each cell clips the same wave, with its origin relative to that cell.
    const hosts = (
      action instanceof HTMLTableRowElement ? Array.from(action.cells) : [action]
    ).filter(
      (host) => host.getBoundingClientRect().width > 0 && host.getBoundingClientRect().height > 0
    );
    if (!hosts.length) return;
    const layers = hosts.map((host) => {
      const bounds = host.getBoundingClientRect();
      const localX =
        (event.clientX - bounds.left) * (host.offsetWidth / bounds.width) - host.clientLeft;
      const localY =
        (event.clientY - bounds.top) * (host.offsetHeight / bounds.height) - host.clientTop;
      const layer = document.createElement('span');
      layer.className = 'touch-ripple';
      layer.setAttribute('aria-hidden', 'true');
      const circle = document.createElement('span');
      circle.className = 'touch-ripple-circle';
      circle.style.width = circle.style.height = `${radius * 2}px`;
      circle.style.left = `${localX - radius}px`;
      circle.style.top = `${localY - radius}px`;
      layer.append(circle);
      const positionHost = getComputedStyle(host).position === 'static';
      if (positionHost) host.setAttribute('data-touch-ripple-host', '');
      host.append(layer);
      const animation = circle.animate(
        [
          { transform: 'scale(0)', opacity: 0.2 },
          { transform: 'scale(1)', opacity: 0 }
        ],
        { duration: 420, easing: 'cubic-bezier(0.2, 0, 0, 1)' }
      );
      return { host, layer, positionHost, animation };
    });
    const dispose = () => {
      for (const { host, layer, positionHost, animation } of layers) {
        animation.onfinish = null;
        animation.cancel();
        layer.remove();
        if (positionHost) host.removeAttribute('data-touch-ripple-host');
      }
      if (cancelRipple === dispose) cancelRipple = undefined;
    };
    cancelRipple = dispose;
    for (const { animation } of layers) animation.onfinish = dispose;
  }

  function start(event: PointerEvent) {
    clear();
    if (!event.isPrimary || event.button !== 0) return;
    const target = event.target;
    if (!(target instanceof Element)) return;
    if (
      target.closest(
        '[contenteditable]:not([contenteditable="false"]), :disabled, [aria-disabled="true"], [data-press-feedback="none"], [inert]'
      )
    )
      return;
    let candidate = target.closest(ACTION);
    const tableBody = target.closest<HTMLElement>('tbody[data-table-row-links]');
    if (tableBody) {
      const rowLink = rowLinkForTarget(tableBody, target);
      if (rowLink) candidate = rowLink.closest('tr');
      else if (candidate?.matches('a.data-table-row-link[href]'))
        candidate = candidate.closest('tr');
    }
    // Native inputs cannot contain the decorative layer. Use their wrapping
    // label when present; standalone inputs keep the fade and push-in only.
    let action =
      candidate instanceof HTMLInputElement ? (candidate.closest('label') ?? candidate) : candidate;
    if (!action || action.matches('label:has(input:disabled)')) return;
    // Only the opted-in action uses its enclosing surface. Nested actions
    // retain their own feedback and never fade or move the whole card.
    if (action.getAttribute('data-press-feedback') === 'surface') {
      action = action.closest('[data-press-surface]') ?? action;
    }
    if (action instanceof HTMLElement && !(action instanceof HTMLInputElement))
      ripple(action, event);
    pressed = action;
    if (getComputedStyle(action).display === 'inline') action.setAttribute('data-press-inline', '');
    pointerId = event.pointerId;
    startX = event.clientX;
    startY = event.clientY;
    pressed.setAttribute('data-touch-pressed', '');
  }

  function move(event: PointerEvent) {
    if (event.pointerId !== pointerId) return;
    // Allow finger jitter; remove the cue before a scroll or drag takes over.
    if (Math.hypot(event.clientX - startX, event.clientY - startY) >= 10) clear();
  }

  function end(event: PointerEvent) {
    if (event.pointerId === pointerId) release();
  }

  function cancel(event: PointerEvent) {
    if (event.pointerId === pointerId) clear();
  }

  // The DOM attribute belongs to this root component's lifetime.
  $effect(() => clear);
</script>

<svelte:document
  onpointerdowncapture={start}
  onpointermovecapture={move}
  onpointerupcapture={end}
  onpointercancelcapture={cancel}
/>
<svelte:window onblur={clear} />
