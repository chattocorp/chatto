import type { PublicAuthProvider } from '$lib/api-client/server';
import { getClientConfiguration } from '$lib/clientConfig';

/** Find the server provider that uses this frontend's trusted Authling issuer. */
export async function findAuthlingServerProvider(
  providers: PublicAuthProvider[]
): Promise<PublicAuthProvider | null> {
  const configuration = await getClientConfiguration();
  if (!configuration.authling) return null;

  const trustedIssuer = new URL(configuration.authling.issuer).origin;
  return (
    providers.find((provider) => {
      if (provider.type !== 'oidc' || !provider.issuerUrl) return false;
      try {
        return new URL(provider.issuerUrl).origin === trustedIssuer;
      } catch {
        return false;
      }
    }) ?? null
  );
}
