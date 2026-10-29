import { afterEach, expect, it, vi } from 'vitest';
import { fadePageContent } from './pageFade';

let stop = () => {};
let root: HTMLDivElement;
afterEach(() => {
  stop();
  root?.remove();
  vi.restoreAllMocks();
});

function fixture(reduced = false) {
  const motion = Object.assign(new EventTarget(), { matches: reduced });
  vi.spyOn(window, 'matchMedia').mockReturnValue(motion as MediaQueryList);
  root = document.createElement('div');
  root.dataset.navigationContent = '';
  root.innerHTML = '<main data-navigation-content>Messages</main><aside>Members</aside>';
  document.body.append(root);
  return { motion, main: root.querySelector('main')! };
}

it('fades only the main content, without transforms or layout changes', () => {
  const { main } = fixture();
  const bounds = main.getBoundingClientRect().toJSON();
  stop = fadePageContent();
  expect(root.getAnimations()).toHaveLength(0);
  expect(root.querySelector('aside')!.getAnimations()).toHaveLength(0);
  const animation = main.getAnimations()[0];
  expect(animation.effect!.getTiming().duration).toBe(140);
  const frames = (animation.effect as KeyframeEffect).getKeyframes();
  expect(frames.map((frame) => frame.opacity)).toEqual(['0.55', '1']);
  expect(frames.every((frame) => !('transform' in frame) && !('scale' in frame))).toBe(true);
  expect(main.getBoundingClientRect().toJSON()).toEqual(bounds);
  stop();
  expect(main.getAnimations()).toHaveLength(0);
});

it('skips reduced motion and cancels when the preference changes', () => {
  const { motion, main } = fixture(true);
  stop = fadePageContent();
  expect(main.getAnimations()).toHaveLength(0);
  motion.matches = false;
  stop = fadePageContent();
  motion.matches = true;
  motion.dispatchEvent(new Event('change'));
  expect(main.getAnimations()).toHaveLength(0);
});

it('uses the page wrapper when there is no nested room pane', () => {
  fixture();
  root.innerHTML = 'Settings';
  stop = fadePageContent();
  expect(root.getAnimations()).toHaveLength(1);
});

it('preserves the dimmed room opacity when navigation opens a thread overlay', () => {
  const { main } = fixture();
  main.style.opacity = '0.3';
  stop = fadePageContent();
  const frames = (main.getAnimations()[0].effect as KeyframeEffect).getKeyframes();
  expect(Number(frames[0].opacity)).toBeCloseTo(0.165);
  expect(Number(frames[1].opacity)).toBeCloseTo(0.3);
});
