// Enhance ordinary links and sign-in; OAuth and other forms retain native navigation.
import './app.css';
import htmx from 'htmx.org';

const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');

Object.assign(htmx.config, {
  allowEval: false,
  allowScriptTags: false,
  includeIndicatorStyles: false,
  historyCacheSize: 0,
  historyRestoreAsHxRequest: false,
  selfRequestsOnly: true,
  globalViewTransitions: !reducedMotion.matches,
  defaultFocusScroll: false,
  reportValidityOfForms: true
});

// Respect preference changes without requiring a document reload.
reducedMotion.addEventListener('change', () => {
  htmx.config.globalViewTransitions = !reducedMotion.matches;
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
    (!url.search || url.pathname === '/admin') &&
    /^(\/|\/login|\/signup|\/password-reset|\/account(?:\/(?:profile|email|password|delete))?|\/admin|\/dev\/ui)$/.test(
      url.pathname
    )
  );
}

document.addEventListener('htmx:beforeProcessNode', (event) => {
  const element = event.target;
  if (
    (element instanceof HTMLFormElement &&
      !(
        element.getAttribute('action') === '/login' && element.getAttribute('hx-boost') === 'true'
      )) ||
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
  // Render login validation and availability errors without replaying the POST
  // or losing the server's message through a fallback GET.
  if (
    event.detail.requestConfig.elt instanceof HTMLFormElement &&
    event.detail.requestConfig.elt.getAttribute('action') === '/login' &&
    response.querySelector('form[action="/login"]')
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
  const target = document.querySelector('main [autofocus]') || document.querySelector('main');
  target?.focus({ preventScroll: true });
}

document.addEventListener('htmx:afterSettle', focusPage);
document.addEventListener('htmx:historyRestore', focusPage);

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
    let notice = document.getElementById('login-network-error');
    if (!notice) {
      notice = document.createElement('p');
      notice.id = 'login-network-error';
      notice.className = 'notice notice-error';
      notice.setAttribute('role', 'alert');
      event.detail.elt.before(notice);
    }
    notice.textContent = "We couldn't reach the server. Please try signing in again.";
  }
});
