import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { attachFogGradients } from './fogGradients';

let motion: EventTarget & { matches: boolean };
let frames: Map<number, FrameRequestCallback>;
let cleanups: (() => void)[];
let nodes: HTMLElement[];

function addFog(): HTMLElement {
  const node = document.createElement('div');
  document.body.append(node);
  nodes.push(node);
  cleanups.push(attachFogGradients(node));
  return node;
}

function advance(time: number): void {
  const callbacks = [...frames.values()];
  frames.clear();
  for (const callback of callbacks) callback(time);
}

beforeEach(() => {
  motion = Object.assign(new EventTarget(), { matches: false });
  vi.spyOn(window, 'matchMedia').mockReturnValue(motion as MediaQueryList);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  frames = new Map();
  cleanups = [];
  nodes = [];
  let id = 0;
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
    frames.set(++id, callback);
    return id;
  });
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((id) => {
    frames.delete(id);
  });
});

afterEach(() => {
  for (const cleanup of cleanups) cleanup();
  for (const node of nodes) node.remove();
  vi.restoreAllMocks();
});

it('gives each block a different noise path while sharing one frame', () => {
  const random = vi.spyOn(Math, 'random').mockReturnValue(0.1);
  const first = addFog();
  random.mockReturnValue(0.8);
  const second = addFog();
  expect(first.style.cssText).not.toBe(second.style.cssText);
  expect(frames.size).toBe(1);

  const initial = first.style.cssText;
  advance(0);
  advance(1000);
  expect(first.style.cssText).not.toBe(initial);
  expect(frames.size).toBe(1);
});

it('pauses for reduced motion and hidden tabs, then cleans up', () => {
  motion.matches = true;
  const fog = addFog();
  const initial = fog.style.cssText;
  expect(frames.size).toBe(0);

  motion.matches = false;
  motion.dispatchEvent(new Event('change'));
  advance(0);
  advance(1000);
  expect(fog.style.cssText).not.toBe(initial);

  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
  document.dispatchEvent(new Event('visibilitychange'));
  const paused = fog.style.cssText;
  expect(frames.size).toBe(0);
  advance(2000);
  expect(fog.style.cssText).toBe(paused);

  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  document.dispatchEvent(new Event('visibilitychange'));
  expect(frames.size).toBe(1);
  cleanups.pop()?.();
  expect(frames.size).toBe(0);
});
