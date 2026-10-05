/** A realtime socket that never opens, for tests that do not use realtime. */
export function inertRealtimeSocket() {
  return {
    binaryType: 'arraybuffer' as BinaryType,
    readyState: 0,
    onopen: null,
    onmessage: null,
    onerror: null,
    onclose: null,
    send() {},
    close() {}
  };
}
