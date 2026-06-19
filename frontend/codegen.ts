import { CodegenConfig } from '@graphql-codegen/cli';

const config: CodegenConfig = {
  schema: '../cli/internal/graph/*.graphqls',
  documents: ['src/**/*.ts', 'src/**/*.svelte'],
  ignoreNoDocuments: true,
  generates: {
    './src/lib/gql/': {
      preset: 'client',
      config: {
        useTypeImports: true
      },
      plugins: []
    }
  }
};

export default config;
