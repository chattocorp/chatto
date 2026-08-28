import type { Preview, Decorator } from '@storybook/sveltekit';
import { themes } from 'storybook/theming';
import '../src/app.css';
import './storybook.css';

const prefersDark =
  typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches;

function resolveTheme(theme: string | undefined): 'light' | 'dark' {
  if (theme === 'light' || theme === 'dark') return theme;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

const previewDecorator: Decorator = (story, context) => {
  if (typeof document !== 'undefined') {
    const resolved = resolveTheme(context.globals.theme as string | undefined);
    document.documentElement.dataset.theme = resolved;
    document.documentElement.dir = context.globals.direction === 'rtl' ? 'rtl' : 'ltr';
  }
  return story();
};

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i
      }
    },
    backgrounds: { disable: true },
    a11y: {
      test: 'error'
    },
    docs: {
      // Storybook themes its own docs chrome (page bg, headings, props
      // table). Pick the preset that matches the user's OS preference at
      // load time so the chrome blends with the manager UI. The story
      // embed inside still flips with the Theme toolbar via the decorator.
      theme: prefersDark ? themes.dark : themes.light
    },
    options: {
      storySort: {
        order: ['Foundations', 'UI', 'Form', 'Admin', '*']
      }
    }
  },
  globalTypes: {
    theme: {
      description: 'Theme',
      toolbar: {
        title: 'Theme',
        icon: 'paintbrush',
        items: [
          { value: 'auto', title: 'Auto (system)', icon: 'mirror' },
          { value: 'light', title: 'Light', icon: 'sun' },
          { value: 'dark', title: 'Dark', icon: 'moon' }
        ],
        dynamicTitle: true
      }
    },
    direction: {
      description: 'Text direction',
      toolbar: {
        title: 'Direction',
        icon: 'globe',
        items: [
          { value: 'ltr', title: 'Left to right' },
          { value: 'rtl', title: 'Right to left' }
        ],
        dynamicTitle: true
      }
    }
  },
  initialGlobals: {
    theme: 'auto',
    direction: 'ltr'
  },
  decorators: [previewDecorator]
};

export default preview;
