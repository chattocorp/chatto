// @vitest-environment node
import { expect, it, vi } from 'vitest';
import { debugLog, setDebugLogging } from './debugLog.js';

it('writes no debug output in Node hosts until enabled', () => {
  const debug = vi.spyOn(console, 'debug').mockImplementation(() => {});
  try {
    debugLog('[test] event dispatched');
    expect(debug).not.toHaveBeenCalled();
    setDebugLogging(true);
    debugLog('[test] event dispatched');
    expect(debug).toHaveBeenCalledOnce();
  } finally {
    setDebugLogging(false);
    debug.mockRestore();
  }
});
