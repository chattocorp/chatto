import { Capacitor, registerPlugin } from '@capacitor/core';
import { oauthPopupResponseFromURL, type OAuthPopupResponse } from '$lib/oauth/popup';

/** The official mobile client has one exact OAuth identity and callback. */
export const MOBILE_CLIENT_ID = 'eu.chattocorp.chatto.mobile';
export const MOBILE_CALLBACK = 'eu.chattocorp.chatto.mobile:/oauth/callback';

const nativeAuthorization = registerPlugin<{
  authorize(options: { url: string }): Promise<{ url: string }>;
}>('ChattoAuthorization');

/** Detect the narrow authentication capability, independently of other host features. */
export function hasNativeAuthorization(): boolean {
  return Capacitor.isPluginAvailable('ChattoAuthorization');
}

/** Reject unrelated callbacks before an authorization code can reach the token endpoint. */
export function validateNativeCallback(callback: string, state: string): OAuthPopupResponse {
  const url = new URL(callback);
  if (
    `${url.protocol}${url.pathname}` !== MOBILE_CALLBACK ||
    url.host ||
    url.username ||
    url.password ||
    url.hash ||
    url.searchParams.getAll('state').length !== 1 ||
    url.searchParams.getAll('code').length > 1 ||
    url.searchParams.getAll('error').length > 1
  ) {
    throw new Error('Invalid sign-in callback.');
  }
  const response = oauthPopupResponseFromURL(url);
  if (!response || response.state !== state || (response.code && response.error)) {
    throw new Error('Invalid sign-in callback.');
  }
  return response;
}

/** Native code owns presentation, cancellation, and the bounded session lifetime. */
export async function authorizeNatively(url: string, state: string): Promise<OAuthPopupResponse> {
  const result = await nativeAuthorization.authorize({ url });
  if (typeof result?.url !== 'string') throw new Error('Invalid sign-in callback.');
  return validateNativeCallback(result.url, state);
}
