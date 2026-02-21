import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import "@xterm/xterm/css/xterm.css";
import "vue-sonner/style.css";
import "./style.css";



const app = createApp(App);
app.use(router);
app.mount("#app");
