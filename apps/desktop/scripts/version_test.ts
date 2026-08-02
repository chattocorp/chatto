import assert from "node:assert/strict";
import { macOSVersions } from "./version.ts";

Deno.test("maps desktop releases to macOS bundle versions", () => {
  assert.deepEqual(macOSVersions("0.1.0"), {
    shortVersion: "0.1.0",
    bundleVersion: "0.1.0",
  });
  assert.deepEqual(macOSVersions("0.1.0-alpha.4"), {
    shortVersion: "0.1.0",
    bundleVersion: "0.1.0a4",
  });
  assert.deepEqual(macOSVersions("0.1.0-beta.2"), {
    shortVersion: "0.1.0",
    bundleVersion: "0.1.0b2",
  });
  assert.deepEqual(macOSVersions("0.1.0-rc.3"), {
    shortVersion: "0.1.0",
    bundleVersion: "0.1.0fc3",
  });
  assert.deepEqual(macOSVersions("0.1.0-dev"), {
    shortVersion: "0.1.0",
    bundleVersion: "0.1.0d1",
  });
});

Deno.test("rejects invalid desktop versions", () => {
  assert.throws(() => macOSVersions("desktop-dev"), TypeError);
});
