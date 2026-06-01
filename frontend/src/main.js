import { createApp } from "vue";
import { createPinia } from "pinia";
import { router } from "./router.js";
import App from "./App.vue";
import "./style.css";

createApp(App).use(createPinia()).use(router).mount("#app");
