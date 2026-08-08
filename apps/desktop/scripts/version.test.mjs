import assert from "node:assert/strict";
import test from "node:test";
import { releaseBuildVersion } from "./version.mjs";

test("converts stable and prerelease SemVer to numeric build versions", () => {
  assert.equal(releaseBuildVersion("1.2.3"), "1.2.3");
  assert.equal(releaseBuildVersion("0.1.0-alpha.4"), "0.1.0.4");
});

test("rejects versions the packaging metadata cannot represent", () => {
  assert.throws(() => releaseBuildVersion("desktop-next"), TypeError);
});
