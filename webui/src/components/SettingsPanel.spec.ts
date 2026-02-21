import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import SettingsPanel from "./SettingsPanel.vue";

describe("SettingsPanel", () => {
  function render() {
    return mount(SettingsPanel, {
      props: {
        show: true,
        defaultLaunchProgram: "shell",
        defaultHelperProgram: "codex",
        providers: [
          { id: "codex", display_name: "codex", command: "codex" },
          { id: "claude", display_name: "Claude", command: "claude" },
          { id: "cursor", display_name: "Cursor", command: "cursor" }
        ],
        taskCompletionMode: "none",
        taskCompletionCommand: "",
        taskCompletionIdleDuration: 10,
        helperOpenaiEndpoint: "",
        helperOpenaiModel: "",
        helperOpenaiApiKey: ""
      },
      global: {
        stubs: {
          ResponsiveOverlay: {
            props: ["open"],
            emits: ["update:open"],
            template: "<div data-test-id='shellman-settings-overlay'><slot /></div>"
          },
          Select: { template: "<div><slot /></div>" },
          SelectTrigger: { template: "<div><slot /></div>" },
          SelectValue: { template: "<div><slot /></div>" },
          SelectContent: { template: "<div><slot /></div>" },
          SelectItem: { template: "<button v-bind='$attrs' @click=\"$emit('click')\"><slot /></button>" },
          Checkbox: {
            props: ["modelValue"],
            emits: ["update:modelValue"],
            template: "<button v-bind='$attrs' @click=\"$emit('update:modelValue', !modelValue)\" />"
          },
          InputGroup: { template: "<div><slot /></div>" },
          InputGroupAddon: { template: "<div><slot /></div>" },
          InputGroupTextarea: {
            props: ["modelValue"],
            emits: ["update:modelValue"],
            template:
              "<textarea " +
              "v-bind='$attrs' " +
              ":value='modelValue' " +
              "@input=\"$emit('update:modelValue', $event.target.value)\" />"
          },
          Button: { template: "<button @click=\"$emit('click')\"><slot /></button>" },
          Input: {
            props: ["modelValue", "type"],
            emits: ["update:modelValue"],
            template:
              "<input " +
              "v-bind='$attrs' " +
              ":value='modelValue' " +
              ":type='type || \"text\"' " +
              "@input=\"$emit('update:modelValue', $event.target.value)\" />"
          }
        }
      }
    });
  }

  it("emits command mode payload with selected delay preset", async () => {
    const wrapper = render();
    await wrapper.setProps({ defaultLaunchProgram: "codex" });
    await wrapper.get("[data-test-id='shellman-settings-task-completion-enable']").trigger("click");
    await wrapper.get("[data-test-id='shellman-settings-task-completion-action-input']").setValue("notify --flag");
    await wrapper.get("[data-test-id='shellman-settings-task-completion-delay-60']").trigger("click");
    await wrapper.get("[data-test-id='shellman-settings-save']").trigger("click");

    expect(wrapper.emitted("save")?.[0]).toEqual([{
      defaultLaunchProgram: "codex",
      defaultHelperProgram: "codex",
      taskCompletionMode: "command",
      taskCompletionCommand: "notify --flag",
      taskCompletionIdleDuration: 60,
      helperOpenAIEndpoint: "",
      helperOpenAIModel: "",
      helperOpenAIApiKey: ""
    }]);
  });

  it("disables command mode and emits none payload on save", async () => {
    const wrapper = render();
    await wrapper.setProps({ taskCompletionMode: "command" });
    await wrapper.get("[data-test-id='shellman-settings-task-completion-enable']").trigger("click");
    await wrapper.get("[data-test-id='shellman-settings-save']").trigger("click");

    expect(wrapper.emitted("save")?.[0]).toEqual([{
      defaultLaunchProgram: "shell",
      defaultHelperProgram: "codex",
      taskCompletionMode: "none",
      taskCompletionCommand: "",
      taskCompletionIdleDuration: 10,
      helperOpenAIEndpoint: "",
      helperOpenAIModel: "",
      helperOpenAIApiKey: ""
    }]);
  });

  it("supports none mode and sends empty command", async () => {
    const wrapper = render();
    await wrapper.get("[data-test-id='shellman-settings-save']").trigger("click");
    expect(wrapper.emitted("save")?.[0]).toEqual([{
      defaultLaunchProgram: "shell",
      defaultHelperProgram: "codex",
      taskCompletionMode: "none",
      taskCompletionCommand: "",
      taskCompletionIdleDuration: 10,
      helperOpenAIEndpoint: "",
      helperOpenAIModel: "",
      helperOpenAIApiKey: ""
    }]);
  });

  it("emits helper openai config from settings", async () => {
    const wrapper = render();
    await wrapper.get("[data-test-id='shellman-settings-helper-openai-endpoint']").setValue("https://api.openai.com");
    await wrapper.get("[data-test-id='shellman-settings-helper-openai-model']").setValue("gpt-5");
    await wrapper.get("[data-test-id='shellman-settings-helper-openai-apikey']").setValue("sk-test-123");
    await wrapper.get("[data-test-id='shellman-settings-save']").trigger("click");

    expect(wrapper.emitted("save")?.[0]?.[0]).toMatchObject({
      helperOpenAIEndpoint: "https://api.openai.com",
      helperOpenAIModel: "gpt-5",
      helperOpenAIApiKey: "sk-test-123"
    });
  });

  it("resets helper openai fields from props whenever panel reopens", async () => {
    const wrapper = render();
    await wrapper.setProps({
      helperOpenaiEndpoint: "https://openrouter.ai/api/v1",
      helperOpenaiModel: "openai/gpt-5.2"
    });
    const endpointInput = wrapper.get("[data-test-id='shellman-settings-helper-openai-endpoint']");
    const modelInput = wrapper.get("[data-test-id='shellman-settings-helper-openai-model']");

    await endpointInput.setValue("");
    await modelInput.setValue("");

    await wrapper.setProps({ show: false });
    await wrapper.setProps({ show: true });

    expect((endpointInput.element as HTMLInputElement).value).toBe("https://openrouter.ai/api/v1");
    expect((modelInput.element as HTMLInputElement).value).toBe("openai/gpt-5.2");
  });
});
