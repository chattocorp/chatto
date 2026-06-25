type GraphQLErrorLike = {
  message?: unknown;
  extensions?: { code?: unknown };
};

type CombinedErrorLike = {
  message?: unknown;
  graphQLErrors?: GraphQLErrorLike[];
};

const UNAUTHENTICATED_CODE = 'UNAUTHENTICATED';
const AUTHENTICATION_REQUIRED_MESSAGE = 'authentication required';

export function isAuthenticationRequiredError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false;

  const combined = error as CombinedErrorLike;
  const graphQLErrors = combined.graphQLErrors ?? [];
  if (graphQLErrors.some((e) => e.extensions?.code === UNAUTHENTICATED_CODE)) {
    return true;
  }

  if (
    graphQLErrors.some(
      (e) =>
        typeof e.message === 'string' && e.message.toLowerCase() === AUTHENTICATION_REQUIRED_MESSAGE
    )
  ) {
    return true;
  }

  return (
    typeof combined.message === 'string' &&
    combined.message.toLowerCase().includes(AUTHENTICATION_REQUIRED_MESSAGE)
  );
}
