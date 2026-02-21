import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import PaneLaunchForm from "./PaneLaunchForm.vue";

describe("PaneLaunchForm", () => {
  function render() {
    return mount(PaneLaunchForm, {
      props: {
        defaultProgram: "shell",
        providers: [
          { id: "codex", display_name: "codex", command: "codex" },
          { id: "claude", display_name: "Claude", command: "claude" }
        ]
      },
      global: {
        stubs: {
          Select: { template: "<div><slot /></div>" },
          SelectTrigger: { template: "<div><slot /></div>" },
          SelectValue: { template: "<div><slot /></div>" },
          SelectContent: { template: "<div><slot /></div>" },
          SelectItem: { template: "<button v-bind='$attrs'><slot /></button>" },
          Textarea: {
            props: ["modelValue"],
            emits: ["update:modelValue"],
            template: "<textarea v-bind='$attrs' :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\" />"
          },
          Button: { template: "<button v-bind='$attrs' @click=\"$emit('click')\"><slot /></button>" }
        }
      }
    });
  }

  it("renders shell and provider options", () => {
    const wrapper = render();
    const text = wrapper.text();
    expect(text).toContain("shell");
    expect(text).toContain("codex");
    expect(text).toContain("Claude");
  });
});
