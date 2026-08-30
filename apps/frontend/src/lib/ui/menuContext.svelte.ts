import { createContext } from 'svelte';

export type MenuPresentation = 'floating' | 'sheet';

export type MenuContext = {
  presentation: () => MenuPresentation;
  containerRole: () => string;
};

const [getMenuContext, setMenuContext] = createContext<MenuContext>();

/** Provides presentation and semantic context to menu descendants. */
export function provideMenuContext(context: MenuContext): void {
  setMenuContext(context);
}

/** Returns the menu context provided by the owning ContextMenu or review fixture. */
export function useMenuContext(): MenuContext {
  return getMenuContext();
}
