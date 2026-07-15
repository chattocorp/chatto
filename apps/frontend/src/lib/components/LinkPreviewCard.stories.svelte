<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { makeRenderData } from '$lib/render/data';
  import LinkPreviewCard, { LinkPreviewViewData } from './LinkPreviewCard.svelte';

  const { Story } = defineMeta({
    title: 'Components/LinkPreviewCard',
    component: LinkPreviewCard,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component: 'Generic link cards and provider-specific rich embeds used in messages.'
        }
      }
    }
  });

  const socialPost = makeRenderData(
    {
      url: 'https://bsky.app/profile/bdbch.com/post/3mqoxsnse4s2o',
      title: 'Dominik Biedebach (@bdbch.com)',
      description:
        "We launched @tiptap.dev's new Server AI Toolkit today — which allows users to add AI agents to an editor that act like normal users.",
      imageUrl: null,
      siteName: 'Bluesky Social',
      embedType: 'bluesky',
      embedId: 'at://did:plc:bmkptaqvfcwmgom75fmo5oo6/app.bsky.feed.post/3mqoxsnse4s2o',
      socialPost: {
        provider: 'bluesky',
        url: 'https://bsky.app/profile/bdbch.com/post/3mqoxsnse4s2o',
        author: {
          displayName: 'Dominik Biedebach',
          handle: 'bdbch.com',
          avatarUrl:
            'https://cdn.bsky.app/img/avatar/plain/did:plc:bmkptaqvfcwmgom75fmo5oo6/bafkreiawn7ylxsx7t5hgxabzcf3hbw36e74abik53pbtqvdqu7z2s2cqaa'
        },
        text: "We launched @tiptap.dev's new Server AI Toolkit today — which allows users to add AI agents to an editor that act like normal users.",
        publishedAt: '2026-07-15T14:50:19.560Z',
        externalLink: {
          url: 'https://tiptap.dev/blog/release-notes/ai-toolkit-now-in-beta',
          title: 'AI Toolkit, Now in Beta — Plus 100 Free Licenses',
          description: 'Build AI agents that understand and edit documents in place.',
          imageUrl:
            'https://cdn.bsky.app/img/feed_thumbnail/plain/did:plc:bmkptaqvfcwmgom75fmo5oo6/bafkreibnkmz4qnyxa3tg5hvjanbormm72uotpsdifmyuq4gumtycr3qwje'
        },
        images: [],
        quotedPost: {
          provider: 'bluesky',
          url: 'https://bsky.app/profile/tiptap.dev/post/3mqotoolkit',
          author: {
            displayName: 'Tiptap',
            handle: 'tiptap.dev'
          },
          text: 'The Server AI Toolkit is now in beta.',
          images: [
            {
              url: 'https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:bmkptaqvfcwmgom75fmo5oo6/bafkreibnkmz4qnyxa3tg5hvjanbormm72uotpsdifmyuq4gumtycr3qwje',
              alt: 'Tiptap AI Toolkit announcement'
            }
          ]
        }
      }
    },
    LinkPreviewViewData
  );
</script>

<Story name="Bluesky post" asChild>
  <div class="max-w-2xl p-4">
    <LinkPreviewCard preview={socialPost} showDismiss={false} />
  </div>
</Story>
