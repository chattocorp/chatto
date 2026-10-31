// Enhance server-rendered feedback on initial loads and htmx navigation.
// Toasts belong to their page; navigation clears their timers and listeners.
const active = new Map();
const lifetime = 8000;

function mountToasts() {
  for (const [element, dispose] of active) {
    if (!element.isConnected) dispose();
  }
  const pending = [...document.querySelectorAll('[data-toast]:not([data-toast-ready])')];
  if (!pending.length) return;
  let region = document.getElementById('toast-region');
  if (!region) {
    region = document.createElement('section');
    region.id = 'toast-region';
    region.className = 'toast-region';
    region.setAttribute('aria-label', 'Notifications');
    region.setAttribute('aria-live', 'polite');
    document.body.append(region);
  }

  for (const toast of pending) {
    toast.dataset.toastReady = 'true';
    toast.className = 'toast';
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'toast-close';
    close.setAttribute('aria-label', 'Dismiss notification');
    const icon = document.createElement('span');
    icon.className = 'iconify uil--times';
    icon.setAttribute('aria-hidden', 'true');
    close.append(icon);
    toast.append(close);
    region.append(toast);

    const controller = new AbortController();
    const options = { signal: controller.signal };
    let remaining = lifetime;
    let started = 0;
    let timer;
    const dispose = () => {
      clearTimeout(timer);
      controller.abort();
      active.delete(toast);
    };
    const dismiss = () => {
      if (toast.contains(document.activeElement)) {
        document.querySelector('main')?.focus({ preventScroll: true });
      }
      dispose();
      toast.remove();
      if (!region.children.length) region.remove();
    };
    const pause = () => {
      if (timer === undefined) return;
      clearTimeout(timer);
      timer = undefined;
      remaining -= performance.now() - started;
    };
    const resume = () => {
      if (
        controller.signal.aborted ||
        !toast.isConnected ||
        timer !== undefined ||
        document.hidden ||
        toast.matches(':hover') ||
        toast.contains(document.activeElement)
      )
        return;
      started = performance.now();
      timer = setTimeout(dismiss, Math.max(0, remaining));
    };
    toast.addEventListener('pointerenter', pause, options);
    toast.addEventListener('pointerleave', resume, options);
    toast.addEventListener('focusin', pause, options);
    toast.addEventListener('focusout', () => queueMicrotask(resume), options);
    document.addEventListener(
      'visibilitychange',
      () => (document.hidden ? pause() : resume()),
      options
    );
    close.addEventListener('click', dismiss, options);
    active.set(toast, dispose);
    resume();
  }

  // Result flags are one-shot feedback, not permanent page state. Keep the
  // email-change flag when it is needed to show a persistent security warning.
  const url = new URL(location.href);
  const flags =
    url.pathname === '/admin/appearance'
      ? ['saved']
      : url.pathname.startsWith('/account')
        ? [
            'password_changed',
            'email_changed',
            'profile_updated',
            'session_revoked',
            'other_sessions_revoked',
            'session_missing',
            'app_revoked',
            'app_missing'
          ]
        : [];
  for (const flag of flags) {
    if (flag === 'email_changed' && url.searchParams.get('email_notice_failed') === '1') continue;
    url.searchParams.delete(flag);
  }
  if (url.href !== location.href) history.replaceState(history.state, '', url);
}

mountToasts();
document.addEventListener('htmx:afterSettle', mountToasts);
document.addEventListener('htmx:historyRestore', mountToasts);
document.addEventListener('htmx:beforeSwap', (event) => {
  if (event.detail.target !== document.body || !event.detail.shouldSwap) return;
  for (const dispose of active.values()) dispose();
});
