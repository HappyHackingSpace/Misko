import js from "@eslint/js";
import globals from "globals";
import pluginVue from "eslint-plugin-vue";

export default [
  { ignores: ["node_modules/**", "dist/**", "playwright-report/**", "test-results/**"] },
  js.configs.recommended,
  // Correctness rules only; Prettier owns formatting.
  ...pluginVue.configs["flat/essential"],
  {
    languageOptions: {
      ecmaVersion: 2023,
      sourceType: "module",
      globals: { ...globals.browser },
    },
    rules: {
      "vue/multi-word-component-names": "off",
    },
  },
  // Build configuration and browser tests run in Node, not in the page.
  {
    files: ["*.config.js", "tests/**/*.js"],
    languageOptions: { globals: { ...globals.node } },
  },
];
