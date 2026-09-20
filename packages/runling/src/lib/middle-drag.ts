/** Dedicated mouse path: button is 1 on press, buttons uses bit 4 during motion. */
export function middleDrag(
  element: HTMLElement,
  begin: (event: MouseEvent) => ((dx: number, dy: number) => void) | undefined,
  finish: () => void = () => {},
) {
  const host = element.ownerDocument.defaultView!;
  let drag:
    | { x: number; y: number; move: (dx: number, dy: number) => void }
    | undefined;
  const stop = () => {
    if (!drag) return;
    drag = undefined;
    finish();
  };
  const down = (event: MouseEvent) => {
    if (event.button !== 1) return;
    event.preventDefault();
    event.stopPropagation();
    stop();
    const move = begin(event);
    if (move) drag = { x: event.clientX, y: event.clientY, move };
  };
  const move = (event: MouseEvent) => {
    if (!drag) return;
    if (!(event.buttons & 4)) {
      stop();
      return;
    }
    event.preventDefault();
    drag.move(event.clientX - drag.x, event.clientY - drag.y);
  };
  const up = (event: MouseEvent) => {
    if (event.button === 1) stop();
  };
  const auxiliary = (event: MouseEvent) => {
    if (event.button === 1) event.preventDefault();
  };
  const capture = { capture: true };
  element.addEventListener("mousedown", down, capture);
  element.addEventListener("auxclick", auxiliary);
  host.addEventListener("mousemove", move, capture);
  host.addEventListener("mouseup", up, capture);
  host.addEventListener("blur", stop);
  return () => {
    stop();
    element.removeEventListener("mousedown", down, capture);
    element.removeEventListener("auxclick", auxiliary);
    host.removeEventListener("mousemove", move, capture);
    host.removeEventListener("mouseup", up, capture);
    host.removeEventListener("blur", stop);
  };
}
