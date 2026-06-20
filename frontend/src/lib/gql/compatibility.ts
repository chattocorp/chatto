type GraphQLErrorLike = {
  message?: unknown;
};

type CombinedErrorLike = {
  message?: unknown;
  graphQLErrors?: GraphQLErrorLike[];
};

function graphQLErrorMessages(error: unknown): string[] {
  if (!error || typeof error !== 'object') return [];

  const combined = error as CombinedErrorLike;
  const messages = (combined.graphQLErrors ?? [])
    .map((e) => e.message)
    .filter((message): message is string => typeof message === 'string');

  if (typeof combined.message === 'string') {
    messages.push(combined.message);
  }

  return messages;
}

export function isUnsupportedGraphQLFieldError(error: unknown, fieldName: string): boolean {
  const needle = `Cannot query field "${fieldName}"`;
  return graphQLErrorMessages(error).some((message) => message.includes(needle));
}

export function isUnsupportedGraphQLArgumentError(error: unknown, argumentName: string): boolean {
  const needles = [`Unknown argument "${argumentName}"`, `Cannot query field "${argumentName}"`];
  return graphQLErrorMessages(error).some((message) =>
    needles.some((needle) => message.includes(needle))
  );
}

export function isUnsupportedGraphQLInputFieldError(error: unknown, fieldName: string): boolean {
  const needles = [
    `Field "${fieldName}" is not defined by type`,
    `Unknown field "${fieldName}"`,
    `Cannot query field "${fieldName}"`
  ];
  return graphQLErrorMessages(error).some((message) =>
    needles.some((needle) => message.includes(needle))
  );
}
