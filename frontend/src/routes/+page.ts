import type { PageLoad } from './$types';

export const load: PageLoad = ({ url }) => {
  // Pass through welcome query param if present.
  return {
    welcome: url.searchParams.get('welcome') === 'true'
  };
};
