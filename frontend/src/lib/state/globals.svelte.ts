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
  isVisible = $state(
    typeof document !== 'undefined' ? document.visibilityState === 'visible' : true
  );

  /**
   * True when the user is actually present at the app: window focused AND
   * tab visible. Drives read-cursor advancement — we only mark messages
   * read while the user can actually see them. A blur or tab-hide flips
   * this false; refocus / re-show flips it back.
   */
  get isPresent(): boolean {
    return this.isFocused && this.isVisible;
  }

  constructor() {
    if (typeof window !== 'undefined') {
      window.addEventListener('focus', () => {
        this.isFocused = true;
      });
      window.addEventListener('blur', () => {
        this.isFocused = false;
      });
    }
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', () => {
        this.isVisible = document.visibilityState === 'visible';
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
 * Combined width of the Server Gutter (~68px, `left-17`) + Server Sidebar
 * (256px, `md:w-64`/`max-md:w-64`). The mobile sidebars slide off-screen by
 * this amount when fully closed.
 */
export const SIDEBAR_PANEL_WIDTH_PX = 68 + 256;

/**
 * Controls the visibility of the left sidebars (Server Gutter and RoomList).
 * Tracks the user's desktop preference separately from viewport-driven changes,
 * so manual toggles on desktop "stick" across viewport transitions.
 *
 * - Desktop: sidebar follows user preference (open by default)
 * - Mobile: sidebar is always closed unless explicitly opened (overlay)
 * - Resizing back to desktop restores the user's last desktop preference
 *
 * Mobile gestures: when a swipe is in progress, `dragOffset` holds the live
 * finger delta (relative to the open position). Templates should disable
 * CSS transitions while dragging and apply the transform from `progress`.
 */
class SidebarNavState {
  isOpen = $state(true);
  /**
   * Live drag offset in px relative to the *open* position. Negative values
   * shift the panel toward closed; 0 = fully open. `null` means no drag is
   * in progress and CSS transitions should drive the transform.
   */
  dragOffset = $state<number | null>(null);
  private desktopOpen = $state(true);
  private _isMobile = $state(false);
  private dragBaselineOpen = false;

  setMobile(isMobile: boolean) {
    if (this._isMobile === isMobile) return;
    this._isMobile = isMobile;
    // Reset any in-flight gesture when the viewport class changes.
    this.dragOffset = null;

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

  /**
   * Animation progress in [0, 1]. 1 = fully open, 0 = fully closed.
   * Uses `dragOffset` when present (live finger), otherwise mirrors `isOpen`.
   * Only meaningful on mobile; desktop should ignore this.
   */
  get progress(): number {
    if (this.dragOffset !== null) {
      const base = this.dragBaselineOpen ? 0 : -SIDEBAR_PANEL_WIDTH_PX;
      const px = clamp(base + this.dragOffset, -SIDEBAR_PANEL_WIDTH_PX, 0);
      return 1 + px / SIDEBAR_PANEL_WIDTH_PX;
    }
    return this.isOpen ? 1 : 0;
  }

  close() {
    this.isOpen = false;
  }

  startDrag() {
    this.dragBaselineOpen = this.isOpen;
    this.dragOffset = 0;
  }

  updateDrag(deltaPx: number) {
    if (this.dragOffset === null) return;
    this.dragOffset = deltaPx;
  }

  /**
   * Commit a drag. Opens or closes based on final progress and fling velocity
   * (px/ms — positive = rightward, opening). Always clears `dragOffset` so
   * CSS transitions resume.
   */
  endDrag(velocityPxPerMs: number) {
    if (this.dragOffset === null) return;
    const finalProgress = this.progress;
    this.dragOffset = null;

    const VELOCITY_THRESHOLD = 0.5;
    if (velocityPxPerMs > VELOCITY_THRESHOLD) {
      this.isOpen = true;
    } else if (velocityPxPerMs < -VELOCITY_THRESHOLD) {
      this.isOpen = false;
    } else {
      this.isOpen = finalProgress >= 0.5;
    }
  }

  initViewportTracking(breakpoint = '(max-width: 767px)'): () => void {
    const mq = window.matchMedia(breakpoint);
    this.setMobile(mq.matches);

    const handler = (e: MediaQueryListEvent) => this.setMobile(e.matches);
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  }
}

function clamp(v: number, min: number, max: number) {
  return Math.max(min, Math.min(max, v));
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
