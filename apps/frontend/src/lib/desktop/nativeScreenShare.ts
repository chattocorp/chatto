// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

type NativeScreenShareSourceBase = {
  id: string;
  width: number;
  height: number;
  preview: Uint8Array;
};

export type NativeScreenShareWindowSource = NativeScreenShareSourceBase & {
  kind: 'window';
  applicationName: string;
  bundleIdentifier: string;
  title: string;
};

export type NativeScreenShareDisplaySource = NativeScreenShareSourceBase & {
  kind: 'display';
  displayIndex: number;
  isMainDisplay: boolean;
};

export type NativeScreenShareSource =
  NativeScreenShareWindowSource | NativeScreenShareDisplaySource;

type NativeScreenShareSourceResponse = {
  protocolVersion: 1;
  sources: NativeScreenShareSource[];
};

const maximumSourceCount = 64;
const maximumPreviewBytes = 512 * 1024;

export type NativeScreenSharePublisherRequest = {
  sourceId: string;
  livekitUrl: string;
  token: string;
  e2eeKey: string;
};

type ChattoDesktopBridge = {
  screenShare: {
    listSources(): Promise<NativeScreenShareSourceResponse>;
    cancelSourceList?(): void;
    startPublisher(request: NativeScreenSharePublisherRequest): string;
  };
};

declare global {
  interface Window {
    chattoDesktop?: ChattoDesktopBridge;
  }
}

/**
 * Whether the host provides native screen sharing. Web browsers omit this
 * optional capability and continue through their own `getDisplayMedia` picker.
 */
export function isNativeScreenShareAvailable(): boolean {
  if (typeof window === 'undefined') return false;
  const capability = window.chattoDesktop?.screenShare;
  return (
    typeof capability?.listSources === 'function' && typeof capability.startPublisher === 'function'
  );
}

/** List temporary, opaque native capture sources for explicit user selection. */
export async function listNativeScreenShareSources(): Promise<NativeScreenShareSource[]> {
  const screenShare = window.chattoDesktop?.screenShare;
  if (
    typeof screenShare?.listSources !== 'function' ||
    typeof screenShare.startPublisher !== 'function'
  ) {
    throw new Error('Native screen sharing is unavailable.');
  }

  const response = await screenShare.listSources();
  if (
    response.protocolVersion !== 1 ||
    !Array.isArray(response.sources) ||
    response.sources.length > maximumSourceCount
  ) {
    throw new Error('The desktop host returned an unsupported capture-source response.');
  }
  if (!response.sources.every(isNativeScreenShareSource)) {
    throw new Error('The desktop host returned an invalid capture source.');
  }
  return response.sources;
}

/** Cancel a superseded native enumeration without affecting an active share. */
export function cancelNativeScreenShareSourceList(): void {
  if (typeof window === 'undefined') return;
  window.chattoDesktop?.screenShare?.cancelSourceList?.();
}

function isNativeScreenShareSource(value: unknown): value is NativeScreenShareSource {
  if (!value || typeof value !== 'object') return false;
  const source = value as Partial<NativeScreenShareSource>;
  const commonValid =
    typeof source.id === 'string' &&
    source.id.length > 0 &&
    source.id.length <= 128 &&
    (source.kind === 'window' || source.kind === 'display') &&
    Number.isSafeInteger(source.width) &&
    (source.width ?? 0) > 0 &&
    (source.width ?? 0) <= 16384 &&
    Number.isSafeInteger(source.height) &&
    (source.height ?? 0) > 0 &&
    (source.height ?? 0) <= 16384 &&
    source.preview instanceof Uint8Array &&
    source.preview.byteLength <= maximumPreviewBytes;
  if (!commonValid) return false;
  if (source.kind === 'display') {
    return (
      Number.isSafeInteger(source.displayIndex) &&
      (source.displayIndex ?? 0) > 0 &&
      typeof source.isMainDisplay === 'boolean'
    );
  }
  const windowSource = source as Partial<NativeScreenShareWindowSource>;
  return (
    typeof windowSource.applicationName === 'string' &&
    windowSource.applicationName.length <= 4096 &&
    typeof windowSource.bundleIdentifier === 'string' &&
    windowSource.bundleIdentifier.length > 0 &&
    windowSource.bundleIdentifier.length <= 512 &&
    typeof windowSource.title === 'string' &&
    windowSource.title.length <= 4096
  );
}
