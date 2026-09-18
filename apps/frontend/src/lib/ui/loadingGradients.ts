import { simplexNoise2D } from './simplexNoise';

/**
 * Animate the startup shell until it is removed. Hidden tabs and reduced-motion
 * preferences pause the clock. The returned cleanup also supports hot reload.
 */
export function startLoadingGradients(shell: HTMLElement): () => void {
  if (!shell.isConnected) return () => {};

  const seed = Math.floor(Math.random() * 0x100000000);
  const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
  let frame: number | undefined;
  let previousTime: number | undefined;
  let elapsed = 0;
  let stopped = false;

  function paint() {
    // Separate noise paths keep the two clouds independent. Clamp the samples
    // so centres and fade radii stay within the intended visual bounds.
    const sample = (path: number) =>
      Math.max(-1, Math.min(1, simplexNoise2D(elapsed / 10000 + 17.3, path, seed)));
    for (const [name, x, y, path] of [
      ['first', 35, 40, 11],
      ['second', 65, 60, 47]
    ] as const) {
      shell.style.setProperty(`--loading-${name}-x`, `${x + sample(path) * 20}%`);
      shell.style.setProperty(`--loading-${name}-y`, `${y + sample(path + 7) * 20}%`);
      shell.style.setProperty(`--loading-${name}-size`, `${50 + sample(path + 19) * 10}%`);
    }
  }

  function tick(time: number) {
    frame = undefined;
    if (!shell.isConnected) {
      stop();
      return;
    }
    if (previousTime !== undefined) elapsed += time - previousTime;
    previousTime = time;
    paint();
    frame = requestAnimationFrame(tick);
  }

  function syncMotion() {
    if (stopped) return;
    if (frame !== undefined) cancelAnimationFrame(frame);
    frame = undefined;
    previousTime = undefined;
    if (!document.hidden && !motion.matches) frame = requestAnimationFrame(tick);
  }

  // The shell is a direct body child. Observe removal even while animation is
  // paused, without observing changes throughout the application subtree.
  const observer = new MutationObserver(() => {
    if (!shell.isConnected) stop();
  });

  function stop() {
    if (stopped) return;
    stopped = true;
    if (frame !== undefined) cancelAnimationFrame(frame);
    observer.disconnect();
    motion.removeEventListener('change', syncMotion);
    document.removeEventListener('visibilitychange', syncMotion);
  }

  paint();
  shell.dataset.noise = '';
  observer.observe(shell.parentNode!, { childList: true });
  motion.addEventListener('change', syncMotion);
  document.addEventListener('visibilitychange', syncMotion);
  syncMotion();
  return stop;
}
