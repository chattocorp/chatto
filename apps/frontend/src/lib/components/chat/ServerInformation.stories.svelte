<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import {
    evaluateServerCompatibility,
    limitedServerFeatures
  } from '@chatto/client/server/compatibility';
  import ServerInformation from './ServerInformation.svelte';
  import exampleIcon from '../../../../static/icons/icon-192.png?inline';

  const exampleBanner = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630"><rect width="1200" height="630" fill="#28566b"/><circle cx="920" cy="130" r="240" fill="#b8c9bc"/><path d="M0 530 320 160 720 630H0" fill="#163443"/><path d="m400 630 440-350 360 350" fill="#467b85"/></svg>')}`;

  const { Story } = defineMeta({
    title: 'Chat/Server information',
    component: ServerInformation,
    tags: ['autodocs'],
    args: { name: 'Example community', host: 'chat.example.test:8443' }
  });
  /** Use real version evaluation for each example, including custom builds. */
  function example(version: string) {
    return {
      version,
      compatibility: evaluateServerCompatibility({ serverVersion: version }),
      features: limitedServerFeatures(version)
    };
  }
</script>

<Story name="Older server" args={example('0.5.0-beta.11')} />
<Story name="Supported server" args={example('0.5.0-beta.18')} />
<Story
  name="Newer server features"
  args={{
    ...example('0.6.0'),
    frontendVersion: '0.5.0-beta.25',
    frontendFeatures: [
      {
        id: 'example-scheduled-messages',
        name: 'Scheduled messages',
        description: 'Choose when a message is sent. This is an example of a future feature.',
        introducedVersion: '0.6.0'
      }
    ]
  }}
/>
<Story
  name="App and server limitations"
  args={{
    ...example('0.5.0-beta.11'),
    frontendVersion: '0.5.0-beta.25',
    frontendFeatures: [
      {
        id: 'example-backported-feature',
        name: 'Scheduled messages',
        description: 'An example feature supplied by a custom server build.',
        introducedVersion: '0.6.0'
      }
    ]
  }}
/>
<Story
  name="Community profile"
  args={{
    ...example('0.5.0-beta.16'),
    iconUrl: exampleIcon,
    bannerUrl: exampleBanner,
    description: 'A place to share projects and meet other makers.',
    welcomeMessage:
      'Welcome to **Example community**!\n\nIntroduce yourself and share what you are working on.'
  }}
/>
<Story name="Unknown version" args={example('custom-build')} />
<Story name="Cannot connect" args={example('0.5.0-beta.8')} />
<Story
  name="Unreachable"
  args={{
    ...example('0.5.0-beta.11'),
    compatibility: { status: 'unreachable', reason: 'unreachable' },
    features: []
  }}
/>
<Story name="Loading" args={{ ...example(''), loading: true }} />
<Story
  name="Feature catalog loading"
  args={{ ...example('0.6.0'), frontendFeaturesLoading: true }}
/>
<Story
  name="Feature catalog failed"
  args={{
    ...example('0.6.0'),
    frontendFeaturesError: 'Network error. Please try again.',
    onRetryFrontendFeatures: () => {}
  }}
/>
