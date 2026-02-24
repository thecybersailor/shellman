import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import i18n from "./i18n";
import "@xterm/xterm/css/xterm.css";
import "vue-sonner/style.css";
import "./app.css";



const app = createApp(App);
app.use(router);
app.use(i18n);
void router.isReady().then(() => {
  app.mount("#app");
});
