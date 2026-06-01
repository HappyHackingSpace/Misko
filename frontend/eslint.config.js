import js from "@eslint/js";
import globals from "globals";
import pluginVue from "eslint-plugin-vue";

export default [
  { ignores: ["node_modules/**", "dist/**"] },
  js.configs.recommended,
  // Doğruluk kuralları (biçimlendirmeyi Prettier yönetir)
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
];
