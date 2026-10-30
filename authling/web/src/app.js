// Enhance ordinary links and explicitly opted-in forms. Protocol flows stay native.
import './app.css';
import htmx from 'htmx.org';

Object.assign(htmx.config, {
  allowEval: false,
  allowScriptTags: false,
  includeIndicatorStyles: false,
  historyCacheSize: 0,
  historyRestoreAsHxRequest: false,
  selfRequestsOnly: true,
  globalViewTransitions: false,
  defaultFocusScroll: false,
  reportValidityOfForms: true
});

// Only known page routes may use AJAX. Query-bearing authentication links carry
// protocol state and must retain native redirects and document security policy.
function isOrdinaryLink(link) {
  const url = new URL(link.href, location.href);
  return (
    url.origin === location.origin &&
    !url.hash &&
    !link.hasAttribute('download') &&
    (!link.target || link.target === '_self') &&
    (!url.search || url.pathname === '/admin' || url.pathname === '/admin/appearance') &&
    /^(\/|\/login|\/signup|\/password-reset|\/account(?:\/(?:profile|email|password|delete|security|apps|sessions))?|\/admin(?:\/appearance)?|\/dev\/ui)$/.test(
      url.pathname
    )
  );
}

// Keep the form allowlist explicit: protocol redirects and multi-step recovery
// need their own review before they can share a document with another page.
function isOrdinaryForm(element) {
  return (
    element instanceof HTMLFormElement &&
    element.getAttribute('hx-boost') === 'true' &&
    ['/login', '/account/profile', '/admin/appearance'].includes(element.getAttribute('action'))
  );
}

document.addEventListener('htmx:beforeProcessNode', (event) => {
  const element = event.target;
  if (
    (element instanceof HTMLFormElement && !isOrdinaryForm(element)) ||
    (element instanceof HTMLAnchorElement && !isOrdinaryLink(element))
  ) {
    element.setAttribute('hx-boost', 'false');
  }
});

// A deployment can change scripts and styles. Load a fresh document
// rather than combining a new response with the previous asset bundle.
document.addEventListener('htmx:beforeSwap', (event) => {
  const { xhr } = event.detail;
  const response = new DOMParser().parseFromString(xhr.responseText, 'text/html');
  // Render form validation and availability errors without replaying the POST
  // or losing the server's message through a fallback GET.
  if (
    isOrdinaryForm(event.detail.requestConfig.elt) &&
    [...response.forms].some(
      (form) =>
        form.getAttribute('action') === event.detail.requestConfig.elt.getAttribute('action')
    )
  ) {
    event.detail.shouldSwap = true;
    event.detail.isError = false;
  }
  const stylesheet = response.querySelector('link[rel="stylesheet"]');
  if (
    !stylesheet ||
    stylesheet.getAttribute('href') !==
      document.querySelector('link[rel="stylesheet"]').getAttribute('href')
  ) {
    event.preventDefault();
    location.assign(xhr.responseURL);
  }
});

function focusPage() {
  // A person can focus a control before the swap settles. Do not move
  // focus again if the destination already has an active element.
  const active = document.activeElement;
  if (active && active !== document.body && active !== document.documentElement) return;
  const target = document.querySelector('main [autofocus]') || document.querySelector('main');
  target?.focus({ preventScroll: true });
}

document.addEventListener('htmx:afterSettle', focusPage);
document.addEventListener('htmx:historyRestore', focusPage);

document.addEventListener('htmx:beforeRequest', (event) => {
  if (isOrdinaryForm(event.detail.elt)) event.detail.elt.setAttribute('aria-busy', 'true');
});
document.addEventListener('htmx:afterRequest', (event) => {
  if (isOrdinaryForm(event.detail.elt)) event.detail.elt.removeAttribute('aria-busy');
});

// History misses fetch fresh HTML without storing account pages. They bypass
// beforeSwap, so check asset changes and session redirects here as well.
document.addEventListener('htmx:historyCacheMissLoad', (event) => {
  const { xhr, path } = event.detail;
  const response = new DOMParser().parseFromString(xhr.responseText, 'text/html');
  const stylesheet = response.querySelector('link[rel="stylesheet"]');
  if (
    new URL(xhr.responseURL).href !== new URL(path, location.href).href ||
    stylesheet?.getAttribute('href') !==
      document.querySelector('link[rel="stylesheet"]').getAttribute('href')
  ) {
    event.detail.swapSpec.swapStyle = 'none';
    location.replace(xhr.responseURL);
  }
});
document.addEventListener('htmx:historyCacheMissLoadError', () => location.reload());

// Failed GET navigation falls back to the server's ordinary error page.
document.addEventListener('htmx:responseError', (event) => {
  location.assign(event.detail.xhr.responseURL);
});
document.addEventListener('htmx:sendError', (event) => {
  if (event.detail.elt instanceof HTMLAnchorElement) location.assign(event.detail.elt.href);
  if (event.detail.elt instanceof HTMLFormElement) {
    let notice = document.getElementById('form-network-error');
    if (!notice) {
      notice = document.createElement('p');
      notice.id = 'form-network-error';
      notice.className = 'notice notice-error';
      notice.setAttribute('role', 'alert');
      event.detail.elt.before(notice);
    }
    notice.textContent =
      "We couldn't confirm whether your submission reached the server. Check your connection and try again.";
  }
});
