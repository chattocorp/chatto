// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from 'vitest';
import { trustedServiceWorkerScriptUrl } from './trustedScriptUrl';

const policyCacheKey = '__chattoServiceWorkerTrustedTypesPolicy';

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(globalThis, policyCacheKey);
});

describe('trustedServiceWorkerScriptUrl', () => {
  it('returns the URL unchanged when Trusted Types are unavailable', () => {
    expect(trustedServiceWorkerScriptUrl('/service-worker.js')).toBe('/service-worker.js');
  });

  it('uses and caches the named service worker policy', () => {
    const trustedUrl = { toString: () => '/service-worker.js' };
    const createScriptURL = vi.fn().mockReturnValue(trustedUrl);
    const createPolicy = vi.fn().mockReturnValue({ createScriptURL });
    vi.stubGlobal('trustedTypes', { createPolicy });

    expect(trustedServiceWorkerScriptUrl('/service-worker.js')).toBe(trustedUrl);
    expect(trustedServiceWorkerScriptUrl('/service-worker.js')).toBe(trustedUrl);
    expect(createPolicy).toHaveBeenCalledOnce();
    expect(createPolicy).toHaveBeenCalledWith(
      'chatto-service-worker-url',
      expect.objectContaining({ createScriptURL: expect.any(Function) })
    );
    expect(createScriptURL).toHaveBeenCalledTimes(2);
  });
});
