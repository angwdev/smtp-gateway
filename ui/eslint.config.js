import js from '@eslint/js';
import reactHooks from 'eslint-plugin-react-hooks';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['dist', 'shots-out', 'test-results', 'playwright-report'] },
  js.configs.recommended,
  ...tseslint.configs.strict,
  reactHooks.configs.flat.recommended,
  {
    languageOptions: { globals: { ...globals.browser } },
    rules: {
      'no-restricted-imports': ['error', {
        paths: [
          { name: '@mui/icons-material', message: 'Import one icon: @mui/icons-material/Check' },
          { name: '@mui/material', importNames: ['Grid', 'styled'], message: 'Lay out with Tailwind classes; style with sx.' },
          { name: 'dayjs', message: 'Use the helpers in src/lib/format.ts.' },
        ],
      }],
    },
  },
  { files: ['src/lib/format.ts', 'src/lib/dates.ts'], rules: { 'no-restricted-imports': 'off' } },
  { files: ['scripts/**', '*.config.*', 'e2e/**'], languageOptions: { globals: { ...globals.node } } },
);
