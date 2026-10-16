<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import {
    evaluateServerCompatibility,
    limitedServerFeatures
  } from '@chatto/client/server/compatibility';
  import ServerInformation from './ServerInformation.svelte';
  import exampleIcon from '../../../../static/icons/icon-192.png?inline';

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
<Story name="Supported server" args={example('0.5.0-beta.16')} />
<Story
  name="Community profile"
  args={{
    ...example('0.5.0-beta.16'),
    iconUrl: exampleIcon,
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
