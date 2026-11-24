import js from '@eslint/js';
import globals from 'globals';

export default [
  { ignores: ['web/static/vendor/**', 'node_modules/**'] },
  js.configs.recommended,
  {
    files: ['web/static/**/*.js', 'web/static/**/*.mjs'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: { ...globals.browser },
    },
    rules: {
      eqeqeq: ['error', 'always'],
      'no-var': 'error',
      'prefer-const': 'error',
      'no-unused-vars': ['error', { args: 'after-used', caughtErrors: 'none' }],
    },
  },
  {
    files: ['web/static/**/*.test.mjs'],
    languageOptions: { globals: { ...globals.node } },
  },
  {
    files: ['eslint.config.js'],
    languageOptions: { globals: { ...globals.node } },
  },
];
