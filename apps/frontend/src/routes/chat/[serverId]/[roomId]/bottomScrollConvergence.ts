export type BottomScrollMeasurement = {
  distanceFromBottom: number;
  scrollSize: number;
  viewportSize: number;
};

type ConvergeAtBottomOptions = {
  continueWhile: () => boolean;
  measure: () => BottomScrollMeasurement | null;
  scroll: () => void;
  waitForFrame: () => Promise<void>;
  maxFrames?: number;
  requiredStableFrames?: number;
  tolerancePx?: number;
};

/**
 * Keep scrolling a virtualized list to its bottom until its measurements stop
 * changing. Returns false when superseded or when convergence exceeds the
 * bounded frame budget.
 */
export async function convergeAtBottom({
  continueWhile,
  measure,
  scroll,
  waitForFrame,
  maxFrames = 30,
  requiredStableFrames = 6,
  tolerancePx = 10
}: ConvergeAtBottomOptions): Promise<boolean> {
  let stableFrames = 0;
  let previousScrollSize: number | null = null;
  let previousViewportSize: number | null = null;

  for (let frame = 0; frame < maxFrames && stableFrames < requiredStableFrames; frame++) {
    await waitForFrame();
    if (!continueWhile()) return false;

    scroll();
    const measurement = measure();
    if (!measurement) return false;

    const measurementsUnchanged =
      measurement.scrollSize === previousScrollSize &&
      measurement.viewportSize === previousViewportSize;
    stableFrames =
      measurement.distanceFromBottom < tolerancePx && measurementsUnchanged ? stableFrames + 1 : 0;
    previousScrollSize = measurement.scrollSize;
    previousViewportSize = measurement.viewportSize;
  }

  return stableFrames >= requiredStableFrames;
}
