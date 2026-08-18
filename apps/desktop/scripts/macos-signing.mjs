/** Resolve macOS signing and notarisation options from the build environment. */
export function macOSDistributionOptions(platform, environment) {
  if (platform !== "darwin") {
    return { osxNotarize: undefined, osxSign: undefined };
  }

  const identity = environment.CHATTO_MACOS_SIGN_IDENTITY?.trim() || "-";
  const notaryValues = {
    appleApiIssuer:
      environment.CHATTO_MACOS_NOTARY_API_ISSUER_ID?.trim() || undefined,
    appleApiKey:
      environment.CHATTO_MACOS_NOTARY_API_KEY?.trim() || undefined,
    appleApiKeyId:
      environment.CHATTO_MACOS_NOTARY_API_KEY_ID?.trim() || undefined,
  };
  const requestedNotarisation = Object.values(notaryValues).some(Boolean);

  if (requestedNotarisation && identity === "-") {
    throw new Error(
      "CHATTO_MACOS_SIGN_IDENTITY is required when notarisation is configured.",
    );
  }
  if (
    requestedNotarisation &&
    Object.values(notaryValues).some((value) => !value)
  ) {
    throw new Error(
      "CHATTO_MACOS_NOTARY_API_KEY, CHATTO_MACOS_NOTARY_API_KEY_ID, and CHATTO_MACOS_NOTARY_API_ISSUER_ID must be set together.",
    );
  }

  return {
    osxNotarize: requestedNotarisation ? notaryValues : undefined,
    osxSign: {
      identity,
      identityValidation: identity !== "-",
      optionsForFile: () => ({
        hardenedRuntime: identity !== "-",
      }),
    },
  };
}
