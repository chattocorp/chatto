import assert from "node:assert/strict";
import test from "node:test";
import { macOSDistributionOptions } from "./macos-signing.mjs";

test("uses ad-hoc signing for ordinary macOS builds", () => {
  const options = macOSDistributionOptions("darwin", {});

  assert.equal(options.osxSign.identity, "-");
  assert.equal(options.osxSign.identityValidation, false);
  assert.deepEqual(options.osxSign.optionsForFile(), {
    hardenedRuntime: false,
  });
  assert.equal(options.osxNotarize, undefined);
});

test("configures Developer ID signing and notarisation", () => {
  const options = macOSDistributionOptions("darwin", {
    CHATTO_MACOS_SIGN_IDENTITY: "Developer ID Application: ChattoCorp GmbH",
    CHATTO_MACOS_NOTARY_API_KEY: "/tmp/AuthKey_123.p8",
    CHATTO_MACOS_NOTARY_API_KEY_ID: "KEY123",
    CHATTO_MACOS_NOTARY_API_ISSUER_ID: "issuer-id",
  });

  assert.equal(
    options.osxSign.identity,
    "Developer ID Application: ChattoCorp GmbH",
  );
  assert.equal(options.osxSign.identityValidation, true);
  assert.deepEqual(options.osxSign.optionsForFile(), {
    hardenedRuntime: true,
  });
  assert.deepEqual(options.osxNotarize, {
    appleApiIssuer: "issuer-id",
    appleApiKey: "/tmp/AuthKey_123.p8",
    appleApiKeyId: "KEY123",
  });
});

test("rejects incomplete notarisation credentials", () => {
  assert.throws(
    () =>
      macOSDistributionOptions("darwin", {
        CHATTO_MACOS_SIGN_IDENTITY: "Developer ID Application",
        CHATTO_MACOS_NOTARY_API_KEY: "/tmp/key.p8",
      }),
    /must be set together/,
  );
});

test("does not configure signing on other platforms", () => {
  assert.deepEqual(
    macOSDistributionOptions("linux", {}),
    { osxNotarize: undefined, osxSign: undefined },
  );
});
