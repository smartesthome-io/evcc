import { type Preview, setup } from "@storybook/vue3";
import "bootstrap/dist/css/bootstrap.min.css";
import smoothscroll from "smoothscroll-polyfill";
import setupI18n from "../ui/classic/assets/js/i18n";
import tooltip from "../ui/classic/assets/js/directives/tooltip";
import "../ui/classic/assets/css/app.css";
import { watchThemeChanges } from "../ui/classic/assets/js/theme";

smoothscroll.polyfill();
watchThemeChanges();

// Setup global parameters
export default {
  parameters: {
    // keep the file order, but push the version group to the end
    options: { storySort: { order: ["*", "Version"] } },
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/,
      },
    },
    backgrounds: {
      default: "default",
      values: [
        {
          name: "default",
          value: "var(--evcc-background)",
        },
        {
          name: "box",
          value: "var(--evcc-box)",
        },
      ],
    },
  },
} satisfies Preview;

setup((app) => {
  app.use(setupI18n());
  app.directive("tooltip", tooltip);

  // Mock router-link for Storybook
  app.component("router-link", {
    props: ["to", "activeClass"],
    template: '<a :href="to"><slot /></a>',
  });
});
