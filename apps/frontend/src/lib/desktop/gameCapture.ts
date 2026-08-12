// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

export type GameCaptureSource = {
  id: string;
  kind: 'window';
  applicationName: string;
  bundleIdentifier: string;
  title: string;
  width: number;
  height: number;
};

type GameCaptureSourceResponse = {
  protocolVersion: 1;
  sources: GameCaptureSource[];
};

export type GameCapturePublisherRequest = {
  sourceId: string;
  livekitUrl: string;
  token: string;
  e2eeKey: string;
};

type ChattoDesktopBridge = {
  gameCapture: {
    listSources(): Promise<GameCaptureSourceResponse>;
    startPublisher(request: GameCapturePublisherRequest): string;
  };
};

declare global {
  interface Window {
    chattoDesktop?: ChattoDesktopBridge;
  }
}

/** Whether this renderer has a native desktop game-capture provider. */
export function isGameCaptureAvailable(): boolean {
  return typeof window !== 'undefined' && window.chattoDesktop?.gameCapture != null;
}

/** List temporary, opaque native capture sources for explicit user selection. */
export async function listGameCaptureSources(): Promise<GameCaptureSource[]> {
  const gameCapture = window.chattoDesktop?.gameCapture;
  if (!gameCapture) throw new Error('Native game capture is unavailable.');

  const response = await gameCapture.listSources();
  if (response.protocolVersion !== 1 || !Array.isArray(response.sources)) {
    throw new Error('The desktop host returned an unsupported capture-source response.');
  }
  if (!response.sources.every(isGameCaptureSource)) {
    throw new Error('The desktop host returned an invalid capture source.');
  }
  return response.sources;
}

function isGameCaptureSource(value: unknown): value is GameCaptureSource {
  if (!value || typeof value !== 'object') return false;
  const source = value as Partial<GameCaptureSource>;
  return (
    typeof source.id === 'string' &&
    source.id.length > 0 &&
    source.kind === 'window' &&
    typeof source.applicationName === 'string' &&
    typeof source.bundleIdentifier === 'string' &&
    source.bundleIdentifier.length > 0 &&
    typeof source.title === 'string' &&
    Number.isSafeInteger(source.width) &&
    (source.width ?? 0) > 0 &&
    Number.isSafeInteger(source.height) &&
    (source.height ?? 0) > 0
  );
}
