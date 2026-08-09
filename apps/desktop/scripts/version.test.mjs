import assert from "node:assert/strict";
import test from "node:test";
import { macOSVersions, releaseBuildVersion } from "./version.mjs";

test("converts stable and prerelease SemVer to numeric build versions", () => {
  assert.equal(releaseBuildVersion("1.2.3"), "1.2.3");
  assert.equal(releaseBuildVersion("0.1.0-alpha.4"), "0.1.0.4");
});

test("rejects versions the packaging metadata cannot represent", () => {
  assert.throws(() => releaseBuildVersion("desktop-next"), TypeError);
});

test("maps desktop releases to macOS bundle versions", () => {
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

test("rejects invalid macOS desktop versions", () => {
  assert.throws(() => macOSVersions("desktop-dev"), TypeError);
});
