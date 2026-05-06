/**
 * App-wide global singletons.
 *
 * Small pieces of state that live for the entire app lifetime,
 * are not scoped to an instance or room, and don't use Svelte context.
 */

// ---------------------------------------------------------------------------
// AppState — browser focus tracking
// ---------------------------------------------------------------------------

class AppState {
  isFocused = $state(typeof document !== 'undefined' ? document.hasFocus() : true);

  constructor() {
    if (typeof window !== 'undefined') {
      window.addEventListener('focus', () => {
        this.isFocused = true;
      });
      window.addEventListener('blur', () => {
        this.isFocused = false;
      });
    }
  }
}

export const appState = new AppState();

// ---------------------------------------------------------------------------
// TitleState — centralized page title
// ---------------------------------------------------------------------------

/**
 * Only the root layout renders <title> via <svelte:head>. Pages and components
 * set their desired title segment through this store; when they unmount they
 * clear it so the root layout falls back to just the instance name.
 */
class TitleState {
  pageTitle = $state<string | null>(null);

  setPageTitle(title: string) {
    this.pageTitle = title;
  }

  clearPageTitle() {
    this.pageTitle = null;
  }
}

export const titleState = new TitleState();

// ---------------------------------------------------------------------------
// SidebarNav — sidebar visibility state
// ---------------------------------------------------------------------------

/**
 * Controls the visibility of the left sidebars (SpaceList and RoomList).
 * Tracks the user's desktop preference separately from viewport-driven changes,
 * so manual toggles on desktop "stick" across viewport transitions.
 *
 * - Desktop: sidebar follows user preference (open by default)
 * - Mobile: sidebar is always closed unless explicitly opened (overlay)
 * - Resizing back to desktop restores the user's last desktop preference
 */
class SidebarNavState {
  isOpen = $state(true);
  private desktopOpen = $state(true);
  private _isMobile = $state(false);

  setMobile(isMobile: boolean) {
    if (this._isMobile === isMobile) return;
    this._isMobile = isMobile;

    if (isMobile) {
      this.isOpen = false;
    } else {
      this.isOpen = this.desktopOpen;
    }
  }

  toggle() {
    this.isOpen = !this.isOpen;
    if (!this._isMobile) {
      this.desktopOpen = this.isOpen;
    }
  }

  get isMobile(): boolean {
    return this._isMobile;
  }

  close() {
    this.isOpen = false;
  }

  initViewportTracking(breakpoint = '(max-width: 767px)'): () => void {
    const mq = window.matchMedia(breakpoint);
    this.setMobile(mq.matches);

    const handler = (e: MediaQueryListEvent) => this.setMobile(e.matches);
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  }
}

export const sidebarNav = new SidebarNavState();

// ---------------------------------------------------------------------------
// QuickSwitcher — Cmd+K palette visibility
// ---------------------------------------------------------------------------

/**
 * Controls visibility of the QuickSwitcher palette. The palette itself is
 * mounted once at the root layout level; this singleton lets any component
 * (header buttons, keyboard shortcuts, etc.) request that it open.
 */
class QuickSwitcherState {
  visible = $state(false);

  open() {
    this.visible = true;
  }

  close() {
    this.visible = false;
  }
}

export const quickSwitcher = new QuickSwitcherState();

// ---------------------------------------------------------------------------
// FullscreenVideo — video overlay state
// ---------------------------------------------------------------------------

/**
 * The inline VideoPlayer lives inside a virtua-virtualized list. Native
 * fullscreen (the browser Fullscreen API) would cause virtua to recalculate,
 * unmount the video DOM node, and immediately exit fullscreen. Instead, we
 * render a separate Vidstack player in a fixed overlay outside the list.
 *
 * No OS-level fullscreen is used — the overlay is a CSS full-viewport div.
 */
let _src = $state<string | null>(null);
let _poster = $state<string | null>(null);
let _startTime = $state(0);

export const fullscreenVideo = {
  get isOpen() {
    return _src !== null;
  },
  get src() {
    return _src;
  },
  get poster() {
    return _poster;
  },
  get startTime() {
    return _startTime;
  },

  open(src: string, poster: string | null, startTime: number) {
    _src = src;
    _poster = poster;
    _startTime = startTime;
  },

  close() {
    _src = null;
    _poster = null;
    _startTime = 0;
  }
};
