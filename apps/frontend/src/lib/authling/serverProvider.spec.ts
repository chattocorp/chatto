import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getClientConfigurationMock } = vi.hoisted(() => ({
  getClientConfigurationMock: vi.fn()
}));

vi.mock('$lib/clientConfig', () => ({ getClientConfiguration: getClientConfigurationMock }));

describe('Authling server provider matching', () => {
  beforeEach(() => {
    getClientConfigurationMock.mockReset();
    getClientConfigurationMock.mockResolvedValue({
      version: 1,
      authling: {
        issuer: 'https://id.example',
        clientId: 'https://app.example/oauth/frontend-client-metadata.json'
      }
    });
  });

  it('selects only the OIDC provider with the trusted issuer', async () => {
    const { findAuthlingServerProvider } = await import('./serverProvider');
    await expect(
      findAuthlingServerProvider([
        {
          id: 'other',
          type: 'oidc',
          label: 'Other',
          loginUrl: '/auth/providers/other',
          issuerUrl: 'https://other.example'
        },
        {
          id: 'authling',
          type: 'oidc',
          label: 'Authling',
          loginUrl: '/auth/providers/authling',
          issuerUrl: 'https://id.example/'
        }
      ])
    ).resolves.toMatchObject({ id: 'authling' });
  });

  it('does not let server metadata select a different issuer', async () => {
    const { findAuthlingServerProvider } = await import('./serverProvider');
    await expect(
      findAuthlingServerProvider([
        {
          id: 'claimed-authling',
          type: 'oidc',
          label: 'Authling',
          loginUrl: '/auth/providers/claimed-authling',
          issuerUrl: 'https://evil.example'
        }
      ])
    ).resolves.toBeNull();
  });

  it('returns no provider when this frontend does not configure Authling', async () => {
    getClientConfigurationMock.mockResolvedValue({ version: 1, authling: null });
    const { findAuthlingServerProvider } = await import('./serverProvider');
    await expect(findAuthlingServerProvider([])).resolves.toBeNull();
  });
});
