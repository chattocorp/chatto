type ScrollSnapshot = {
  scrollTop: number;
  timeout: ReturnType<typeof setTimeout>;
};

const pendingSnapshots = new Map<string, ScrollSnapshot>();

function verticalScrollContainer(element: HTMLElement): HTMLElement | null {
  for (let candidate = element.parentElement; candidate; candidate = candidate.parentElement) {
    const overflowY = getComputedStyle(candidate).overflowY;
    if (
      (overflowY === 'auto' || overflowY === 'scroll') &&
      candidate.scrollHeight > candidate.clientHeight
    ) {
      return candidate;
    }
  }
  return null;
}

/** Capture page scroll before an RBAC mutation can remount its management pane. */
export function capturePermissionMutationScroll(context: string, element?: HTMLElement): void {
  cancelPermissionMutationScroll(context);
  if (!element) return;

  const scrollContainer = verticalScrollContainer(element);
  if (!scrollContainer) return;

  const snapshot: ScrollSnapshot = {
    scrollTop: scrollContainer.scrollTop,
    timeout: setTimeout(() => {
      if (pendingSnapshots.get(context) === snapshot) pendingSnapshots.delete(context);
    }, 10_000)
  };
  pendingSnapshots.set(context, snapshot);
}

/** Discard scroll captured for a permission mutation that did not complete. */
export function cancelPermissionMutationScroll(context: string): void {
  const snapshot = pendingSnapshots.get(context);
  if (!snapshot) return;
  clearTimeout(snapshot.timeout);
  pendingSnapshots.delete(context);
}

/** Restore scroll when the authoritative permission matrix mounts after the RBAC reset. */
export function restorePermissionMutationScroll(context: string, element: HTMLElement): void {
  const snapshot = pendingSnapshots.get(context);
  if (!snapshot) return;

  const scrollContainer = verticalScrollContainer(element);
  if (!scrollContainer) return;

  scrollContainer.scrollTop = snapshot.scrollTop;
  clearTimeout(snapshot.timeout);
  pendingSnapshots.delete(context);
}
