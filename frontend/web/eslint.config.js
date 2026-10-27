import antfu from '@antfu/eslint-config'

export default antfu({
  vue: true,
  typescript: true,
  ignores: ['DESIGN.md', 'README.md', 'test-results/**'],
}, {
  rules: {
    // Project idioms the preset disallows: one-line `{ a(); b() }` blocks,
    // helpers declared below their callers, kebab-case emits, `confirm()`
    // prompts, and the `process`/`Buffer` globals.
    'style/max-statements-per-line': 'off',
    'ts/no-use-before-define': 'off',
    'vue/custom-event-name-casing': 'off',
    'no-alert': 'off',
    'node/prefer-global/process': 'off',
    'node/prefer-global/buffer': 'off',
  },
}, {
  files: ['**/*.vue'],
  rules: {
    // Line breaks inside an element become rendered whitespace.
    'vue/singleline-html-element-content-newline': 'off',
    'vue/multiline-html-element-content-newline': 'off',
    // Treats `<script>` and `<script setup>` as one module and hoists across them.
    'import/first': 'off',
  },
}, {
  files: ['**/*.test.ts', 'src/test/**', 'e2e/**'],
  rules: {
    'no-console': 'off',
    'no-sequences': 'off',
    'accessor-pairs': 'off',
    'test/prefer-hooks-in-order': 'off',
  },
})
