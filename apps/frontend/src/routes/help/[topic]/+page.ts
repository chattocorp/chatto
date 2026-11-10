import { error } from '@sveltejs/kit';
import { helpTopics } from '$lib/help/navigation';
import type { PageLoad } from './$types';

/** Unknown articles use the standard not-found page. */
export const load: PageLoad = ({ params }) => {
  const topic = helpTopics.find((candidate) => candidate === params.topic);
  if (!topic) error(404);
  return { topic };
};
