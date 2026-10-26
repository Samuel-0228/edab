import js from '@eslint/js';
import ts from '@typescript-eslint/eslint-plugin';
import tsParser from '@typescript-eslint/parser';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';

export default [
  js.configs.recommended,
  ...svelte.configs['flat/recommended'],
  {
    plugins: {
      '@typescript-eslint': ts
    },
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node
      }
    },
    rules: {
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': [
        'warn',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }
      ],
      'no-undef': 'off',
      'svelte/require-each-key': 'warn',
      'svelte/no-at-html-tags': 'off',
      'svelte/no-unused-svelte-ignore': 'off'
    }
  },
  {
    files: ['**/*.svelte'],
    languageOptions: {
      parserOptions: {
        parser: tsParser,
        extraFileExtensions: ['.svelte']
      }
    }
  },
  {
    ignores: [
      'dist/**',
      'coverage/**',
      'node_modules/**',
      '.svelte-kit/**',
      '*.config.js',
      '*.config.ts'
    ]
  }
];
