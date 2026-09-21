import { expect, test } from "vitest";
import { isSidebarShortcut } from "./sidebar-shortcut.ts";

const key = {
  key: "b",
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  isComposing: false,
  defaultPrevented: false,
};

test("supports Command+B and Control+B, including Caps Lock", () => {
  expect(isSidebarShortcut({ ...key, metaKey: true })).toBe(true);
  expect(isSidebarShortcut({ ...key, ctrlKey: true })).toBe(true);
  expect(isSidebarShortcut({ ...key, metaKey: true, key: "B" })).toBe(true);
});

test("leaves typing, other shortcuts, composition, and handled events alone", () => {
  expect(isSidebarShortcut(key)).toBe(false);
  for (const override of [
    { key: "a" },
    { altKey: true },
    { shiftKey: true },
    { isComposing: true },
    { defaultPrevented: true },
  ]) {
    expect(isSidebarShortcut({ ...key, ctrlKey: true, ...override })).toBe(false);
  }
});
