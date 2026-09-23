/** Send an initial snapshot and live changes. Release listeners when the client leaves. */
export function eventStream(
  request: Request,
  connect: (send: (event: string, data: unknown) => void) => () => void,
): Response {
  let cleanup = () => {};
  const encoder = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      let closed = false;
      const send = (event: string, data: unknown) => {
        if (!closed)
          controller.enqueue(
            encoder.encode(
              `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`,
            ),
          );
      };
      const unsubscribe = connect(send);
      const heartbeat = setInterval(() => {
        if (!closed) controller.enqueue(encoder.encode(": keep-alive\n\n"));
      }, 15_000);
      cleanup = () => {
        if (closed) return;
        closed = true;
        clearInterval(heartbeat);
        unsubscribe();
        request.signal.removeEventListener("abort", abort);
      };
      const abort = () => {
        cleanup();
        controller.close();
      };
      request.signal.addEventListener("abort", abort, { once: true });
      if (request.signal.aborted) abort();
    },
    cancel() {
      cleanup();
    },
  });
  return new Response(body, {
    headers: {
      "content-type": "text/event-stream",
      "cache-control": "no-cache, no-transform",
      "x-accel-buffering": "no",
    },
  });
}
