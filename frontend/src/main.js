import { createApp } from "vue";
import { createPinia } from "pinia";
import { router } from "./router.js";
import { i18n } from "./i18n/index.js";
import { usePrefs } from "./stores/prefs.js";
import App from "./App.vue";
import "./style.css";

const app = createApp(App);
const pinia = createPinia();

app.use(pinia).use(i18n).use(router);

// Apply saved theme + language before mount so there's no flash.
usePrefs(pinia).init();

app.mount("#app");
