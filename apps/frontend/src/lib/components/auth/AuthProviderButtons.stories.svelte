<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AuthProviderButtons from './AuthProviderButtons.svelte';

  const { Story } = defineMeta({
    title: 'Auth/Provider buttons',
    component: AuthProviderButtons,
    args: {
      providers: [
        {
          id: 'github',
          type: 'github',
          label: 'GitHub',
          loginUrl: '/auth/providers/github',
          issuerUrl: null,
          autoProvision: true
        },
        {
          id: 'company',
          type: 'oidc',
          label: 'Company SSO',
          loginUrl: '/auth/providers/company',
          issuerUrl: null,
          autoProvision: true
        }
      ],
      onclick: () => undefined
    }
  });
</script>

<Story name="Default" />
<Story name="Compact" args={{ compact: true }} />
<Story name="Connecting" args={{ selectedProviderId: 'company' }} />
