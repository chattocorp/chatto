import assert from "node:assert/strict";
import test from "node:test";
import { hasAppOrigin, isDesktopPermissionAllowed } from "./security.mjs";

test("recognises only the fixed desktop origin", () => {
  assert.equal(hasAppOrigin("chatto://desktop/chat/room"), true);
  assert.equal(hasAppOrigin("chatto://desktop.evil/chat/room"), false);
  assert.equal(hasAppOrigin("https://desktop/chat/room"), false);
});

test("allows only required permissions for the desktop origin", () => {
  assert.equal(isDesktopPermissionAllowed("media", "chatto://desktop"), true);
  assert.equal(
    isDesktopPermissionAllowed("notifications", "chatto://desktop/login"),
    true,
  );
  assert.equal(
    isDesktopPermissionAllowed("geolocation", "chatto://desktop"),
    false,
  );
  assert.equal(
    isDesktopPermissionAllowed("notifications", "https://chat.example"),
    false,
  );
});
