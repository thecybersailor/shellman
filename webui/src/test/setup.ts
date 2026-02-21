import { config } from "@vue/test-utils";
import i18n from "@/i18n";

const existingPlugins = Array.isArray(config.global.plugins) ? config.global.plugins : [];

if (!existingPlugins.includes(i18n)) {
  config.global.plugins = [...existingPlugins, i18n];
}
