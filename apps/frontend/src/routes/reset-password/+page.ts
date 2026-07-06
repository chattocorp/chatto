export const load = ({ url }) => ({
  token: url.searchParams.get('token')
});
