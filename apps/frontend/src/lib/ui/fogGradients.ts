import { simplexNoise2D } from './simplexNoise';

type Cloud = {
  name: 'first' | 'second' | 'third';
  x: number;
  y: number;
  path: number;
  phase: number;
};

type Fog = { node: HTMLElement; seed: number; clouds: Cloud[] };

const fogs = new Set<Fog>();
let frame: number | undefined;
let motion: MediaQueryList | undefined;
let previousTime: number | undefined;
let previousPaint: number | undefined;
let elapsed = 0;

function paint(fog: Fog): void {
  const time = elapsed / 11000;
  for (const cloud of fog.clouds) {
    const x = simplexNoise2D(time + cloud.phase, cloud.path, fog.seed);
    const y = simplexNoise2D(time + cloud.phase + 9.7, cloud.path + 7, fog.seed);
    fog.node.style.setProperty(`--fog-${cloud.name}-x`, `${cloud.x + x * 14}%`);
    fog.node.style.setProperty(`--fog-${cloud.name}-y`, `${cloud.y + y * 14}%`);
  }
}

function tick(time: number): void {
  frame = undefined;
  if (previousTime !== undefined) elapsed += Math.max(0, time - previousTime);
  previousTime = time;
  // The clouds drift slowly, so 30 paints per second are enough.
  if (previousPaint === undefined || time - previousPaint >= 1000 / 30) {
    for (const fog of fogs) paint(fog);
    previousPaint = time;
  }
  frame = requestAnimationFrame(tick);
}

function syncMotion(): void {
  if (frame !== undefined) cancelAnimationFrame(frame);
  frame = undefined;
  previousTime = undefined;
  previousPaint = undefined;
  if (fogs.size && !document.hidden && !motion?.matches) frame = requestAnimationFrame(tick);
}

/** Give each fog its own simplex-noise path while sharing one animation clock. */
export function attachFogGradients(node: HTMLElement): () => void {
  const seed = Math.floor(Math.random() * 0x100000000);
  const bases: Cloud[] = [
    { name: 'first', x: 30, y: 40, path: 11, phase: 0 },
    { name: 'second', x: 70, y: 60, path: 47, phase: 0 },
    { name: 'third', x: 52, y: 48, path: 83, phase: 0 }
  ];
  const clouds = bases.map((cloud) => ({
    ...cloud,
    x: cloud.x + (Math.random() - 0.5) * 14,
    y: cloud.y + (Math.random() - 0.5) * 14,
    phase: Math.random() * 32
  }));
  for (const cloud of clouds) {
    node.style.setProperty(`--fog-${cloud.name}-size`, `${48 + Math.random() * 8}%`);
  }

  const fog = { node, seed, clouds };
  fogs.add(fog);
  paint(fog);
  if (fogs.size === 1) {
    motion = window.matchMedia('(prefers-reduced-motion: reduce)');
    motion.addEventListener('change', syncMotion);
    document.addEventListener('visibilitychange', syncMotion);
    syncMotion();
  }

  return () => {
    fogs.delete(fog);
    if (fogs.size) return;
    syncMotion();
    motion?.removeEventListener('change', syncMotion);
    document.removeEventListener('visibilitychange', syncMotion);
    motion = undefined;
    elapsed = 0;
  };
}
