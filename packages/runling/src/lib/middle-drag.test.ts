import { expect, test, vi } from "vitest";
import { middleDrag } from "./middle-drag.ts";

function fixture() {
  const host = new EventTarget();
  const element = Object.assign(new EventTarget(), {
    ownerDocument: { defaultView: host },
  }) as unknown as HTMLElement;
  const emit = (target: EventTarget, type: string, values = {}) => {
    const event = Object.assign(
      new Event(type, { cancelable: true }),
      { button: 0, buttons: 0, clientX: 0, clientY: 0 },
      values,
    );
    target.dispatchEvent(event);
    return event;
  };
  return { host, element, emit };
}

test("middle drag uses the held-button bitmask and works outside the target", () => {
  const { host, element, emit } = fixture();
  const move = vi.fn();
  const finish = vi.fn();
  const cleanup = middleDrag(element, () => move, finish);
  expect(
    emit(element, "mousedown", { button: 1, clientX: 50, clientY: 30 })
      .defaultPrevented,
  ).toBe(true);
  emit(host, "mousemove", { buttons: 4, clientX: 70, clientY: 20 });
  expect(move).toHaveBeenCalledWith(20, -10);
  emit(host, "mouseup", { button: 1 });
  expect(finish).toHaveBeenCalledOnce();
  emit(host, "mousemove", { buttons: 4 });
  expect(move).toHaveBeenCalledOnce();
  cleanup();
});

test("leaves other buttons alone and stops on blur or a missed release", () => {
  const { host, element, emit } = fixture();
  const begin = vi.fn(() => vi.fn());
  const finish = vi.fn();
  const cleanup = middleDrag(element, begin, finish);
  emit(element, "mousedown", { button: 0 });
  emit(element, "mousedown", { button: 2 });
  expect(begin).not.toHaveBeenCalled();
  emit(element, "mousedown", { button: 1 });
  emit(host, "blur");
  emit(element, "mousedown", { button: 1 });
  emit(host, "mousemove", { buttons: 0 });
  expect(finish).toHaveBeenCalledTimes(2);
  cleanup();
  emit(element, "mousedown", { button: 1 });
  expect(begin).toHaveBeenCalledTimes(2);
});
