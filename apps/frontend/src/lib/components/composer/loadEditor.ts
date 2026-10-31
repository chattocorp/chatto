import type { Component } from 'svelte';
import type { ComposerEditorKind } from '$lib/state/userPreferences.svelte';
import type { ComposerEditorProps } from './editorTypes';

type EditorModule = { default: Component<ComposerEditorProps> };

const loaders = {
  visual: () => import('./TipTapEditor.svelte'),
  markdown: () => import('./MarkdownEditor.svelte')
};
// Code is shared across fields and sessions; editor instances and drafts are not.
const modules: Partial<Record<ComposerEditorKind, EditorModule | Promise<EditorModule>>> = {};

/** Share pending imports and return loaded code synchronously to skip pending markup on reopen. */
export function loadEditor(kind: ComposerEditorKind): EditorModule | Promise<EditorModule> {
  return (modules[kind] ??= loaders[kind]().then(
    (module) => (modules[kind] = module),
    (error) => {
      delete modules[kind];
      throw error;
    }
  ));
}
