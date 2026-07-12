import { describe, expect, it, vi } from 'vitest';
import { convergeAtBottom, type BottomScrollMeasurement } from './bottomScrollConvergence';

describe('convergeAtBottom', () => {
  it('waits for virtualizer measurements to remain stable', async () => {
    const measurements: BottomScrollMeasurement[] = [
      { distanceFromBottom: 0, scrollSize: 1_000, viewportSize: 400 },
      { distanceFromBottom: 0, scrollSize: 1_000, viewportSize: 400 },
      { distanceFromBottom: 80, scrollSize: 1_080, viewportSize: 400 },
      { distanceFromBottom: 0, scrollSize: 1_080, viewportSize: 400 },
      { distanceFromBottom: 0, scrollSize: 1_080, viewportSize: 400 },
      { distanceFromBottom: 0, scrollSize: 1_080, viewportSize: 400 }
    ];
    let frame = -1;
    const scroll = vi.fn();

    const converged = await convergeAtBottom({
      continueWhile: () => true,
      measure: () => measurements[frame] ?? measurements.at(-1)!,
      scroll,
      waitForFrame: async () => {
        frame += 1;
      },
      requiredStableFrames: 2
    });

    expect(converged).toBe(true);
    expect(scroll).toHaveBeenCalledTimes(5);
  });

  it('stops without scrolling again when superseded', async () => {
    let active = true;
    const scroll = vi.fn();

    const converged = await convergeAtBottom({
      continueWhile: () => active,
      measure: () => ({ distanceFromBottom: 0, scrollSize: 1_000, viewportSize: 400 }),
      scroll,
      waitForFrame: async () => {
        active = false;
      }
    });

    expect(converged).toBe(false);
    expect(scroll).not.toHaveBeenCalled();
  });

  it('returns false when measurements never settle within the frame budget', async () => {
    let frame = 0;

    const converged = await convergeAtBottom({
      continueWhile: () => true,
      measure: () => ({ distanceFromBottom: 0, scrollSize: frame++, viewportSize: 400 }),
      scroll: vi.fn(),
      waitForFrame: async () => {},
      maxFrames: 4,
      requiredStableFrames: 2
    });

    expect(converged).toBe(false);
  });
});
