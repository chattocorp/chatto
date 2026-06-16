type GraphQLErrorLike = {
  message?: unknown;
  extensions?: { code?: unknown };
};

type CombinedErrorLike = {
  message?: unknown;
  graphQLErrors?: GraphQLErrorLike[];
};

export function isAuthenticationRequiredError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false;

  const combined = error as CombinedErrorLike;
  const graphQLErrors = combined.graphQLErrors ?? [];
  if (
    graphQLErrors.some(
      (e) => typeof e.message === 'string' && e.message.toLowerCase() === 'authentication required'
    )
  ) {
    return true;
  }

  return (
    typeof combined.message === 'string' &&
    combined.message.toLowerCase().includes('authentication required')
  );
}
