// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

const SERVICE_WORKER_POLICY_NAME = 'chatto-service-worker-url';
const POLICY_CACHE_KEY = '__chattoServiceWorkerTrustedTypesPolicy';

type TrustedScriptURLValue = string | { toString(): string };

type TrustedTypePolicy = {
  createScriptURL(url: string): TrustedScriptURLValue;
};

type TrustedTypesGlobal = typeof globalThis & {
  trustedTypes?: {
    createPolicy(
      name: string,
      policy: {
        createScriptURL(url: string): string;
      }
    ): TrustedTypePolicy;
  };
  [POLICY_CACHE_KEY]?: TrustedTypePolicy;
};

const trustedTypesGlobal = globalThis as TrustedTypesGlobal;

/**
 * Trusts a reviewed, same-origin service worker script URL. Do not use this
 * policy for operator or user-provided URLs.
 */
export function trustedServiceWorkerScriptUrl(url: string): TrustedScriptURLValue {
  if (!trustedTypesGlobal.trustedTypes) return url;

  trustedTypesGlobal[POLICY_CACHE_KEY] ??= trustedTypesGlobal.trustedTypes.createPolicy(
    SERVICE_WORKER_POLICY_NAME,
    {
      createScriptURL(scriptUrl) {
        return scriptUrl;
      }
    }
  );
  return trustedTypesGlobal[POLICY_CACHE_KEY].createScriptURL(url);
}
