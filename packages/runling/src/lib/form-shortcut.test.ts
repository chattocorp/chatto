import { expect, test, vi } from "vitest";
import { submitFormShortcut } from "./form-shortcut.ts";

function event(overrides: Partial<KeyboardEvent> = {}) {
  return {
    key: "Enter", metaKey: true, ctrlKey: false, altKey: false,
    shiftKey: false, repeat: false, isComposing: false, defaultPrevented: false,
    preventDefault: vi.fn(), currentTarget: { requestSubmit: vi.fn() },
    ...overrides,
  } as unknown as KeyboardEvent;
}

test("Cmd+Enter and Ctrl+Enter use normal form submission", () => {
  for (const modifiers of [{ metaKey: true }, { metaKey: false, ctrlKey: true }]) {
    const key = event(modifiers);
    submitFormShortcut(key);
    expect(key.preventDefault).toHaveBeenCalledOnce();
    expect((key.currentTarget as HTMLFormElement).requestSubmit).toHaveBeenCalledOnce();
  }
});

test("leaves plain Enter, composition, repeats, and other shortcuts alone", () => {
  for (const overrides of [
    { metaKey: false }, { key: "b" }, { isComposing: true },
    { repeat: true }, { altKey: true }, { shiftKey: true }, { defaultPrevented: true },
  ]) {
    const key = event(overrides);
    submitFormShortcut(key);
    expect(key.preventDefault).not.toHaveBeenCalled();
    expect((key.currentTarget as HTMLFormElement).requestSubmit).not.toHaveBeenCalled();
  }
});

test("does not submit again while a request is pending", () => {
  const key = event();
  submitFormShortcut(key, true);
  expect(key.preventDefault).toHaveBeenCalledOnce();
  expect((key.currentTarget as HTMLFormElement).requestSubmit).not.toHaveBeenCalled();
});
