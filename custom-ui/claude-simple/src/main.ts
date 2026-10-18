import { createApp } from "vue";
import App from "./App.vue";
import { connect } from "./state";
import "./style.css";

connect();
createApp(App).mount("#app");
