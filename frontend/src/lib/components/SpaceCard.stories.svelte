<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SpaceCard from './SpaceCard.svelte';

  const { Story } = defineMeta({
    title: 'Components/SpaceCard',
    component: SpaceCard,
    tags: ['autodocs']
  });

  // The `space` prop is typed as `SpaceCardSpaceFragment` (a generated GraphQL
  // fragment). Storybook only needs the shape, so we cast a plain object.
  // The fragment fields used by the component are all simple scalars.
  type SpaceFixture = {
    __typename: 'Space';
    id: string;
    name: string;
    description: string | null;
    logoUrl: string | null;
    bannerUrl: string | null;
    memberCount: number;
    viewerCanJoinSpace: boolean;
    viewerIsMember: boolean;
  };

  const baseSpace: SpaceFixture = {
    __typename: 'Space',
    id: 'SAbcDef',
    name: 'Open Source Hangout',
    description:
      'A friendly community for people who hack on open source projects in their spare time.',
    logoUrl: 'https://picsum.photos/seed/oss-logo/96/96',
    bannerUrl: 'https://picsum.photos/seed/oss-banner/384/288',
    memberCount: 142,
    viewerCanJoinSpace: true,
    viewerIsMember: false
  };
</script>

<Story name="Joined (with banner)" asChild>
  <div class="max-w-md">
    <SpaceCard
      space={{ ...baseSpace, viewerIsMember: true } as never}
      joined
      href="#"
    />
  </div>
</Story>

<Story name="Can join" asChild>
  <div class="max-w-md">
    <SpaceCard space={baseSpace as never} onjoin={() => {}} />
  </div>
</Story>

<Story name="Joining (loading)" asChild>
  <div class="max-w-md">
    <SpaceCard space={baseSpace as never} joining onjoin={() => {}} />
  </div>
</Story>

<Story name="No permission" asChild>
  <div class="max-w-md">
    <SpaceCard
      space={{ ...baseSpace, viewerCanJoinSpace: false } as never}
    />
  </div>
</Story>

<Story name="No banner (gradient + logo)" asChild>
  <div class="max-w-md">
    <SpaceCard
      space={{ ...baseSpace, bannerUrl: null } as never}
      joined
      href="#"
    />
  </div>
</Story>

<Story name="No banner, no logo (gradient only)" asChild>
  <div class="max-w-md">
    <SpaceCard
      space={{ ...baseSpace, bannerUrl: null, logoUrl: null, name: 'Bauhaus Crew' } as never}
      joined
      href="#"
    />
  </div>
</Story>

<Story name="Long description (truncated)" asChild>
  <div class="max-w-md">
    <SpaceCard
      space={{
        ...baseSpace,
        description:
          'A space for serious aficionados of obscure programming languages, paradigms, and ' +
          'historical computing curiosities. We discuss everything from APL to Zig, with ' +
          'occasional detours through forgotten dialects of Lisp, the cultural impact of ' +
          'Smalltalk, and the surprising relevance of 1970s timesharing systems today.'
      } as never}
      joined
      href="#"
    />
  </div>
</Story>

<Story name="Single member" asChild>
  <div class="max-w-md">
    <SpaceCard
      space={{ ...baseSpace, memberCount: 1, name: 'My Private Space' } as never}
      joined
      href="#"
    />
  </div>
</Story>

<Story name="Grid of cards" asChild>
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
    <SpaceCard
      space={{ ...baseSpace, viewerIsMember: true } as never}
      joined
      href="#"
    />
    <SpaceCard space={{ ...baseSpace, id: '2', name: 'Plant Parents', bannerUrl: 'https://picsum.photos/seed/plants/384/288', memberCount: 23 } as never} onjoin={() => {}} />
    <SpaceCard
      space={{ ...baseSpace, id: '3', name: 'Brutalist Architecture', bannerUrl: null, memberCount: 89 } as never}
      onjoin={() => {}}
    />
    <SpaceCard
      space={{ ...baseSpace, id: '4', name: 'Tarot & Tea', bannerUrl: 'https://picsum.photos/seed/tarot/384/288', memberCount: 7 } as never}
      onjoin={() => {}}
    />
    <SpaceCard
      space={{ ...baseSpace, id: '5', name: 'Cycling Touring', bannerUrl: 'https://picsum.photos/seed/cycling/384/288', memberCount: 451, description: null } as never}
      onjoin={() => {}}
    />
    <SpaceCard
      space={{ ...baseSpace, id: '6', name: 'Dimly Lit Bars', bannerUrl: null, logoUrl: null, memberCount: 12, viewerCanJoinSpace: false } as never}
    />
  </div>
</Story>
