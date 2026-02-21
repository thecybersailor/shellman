import { describe, expect, it } from "vitest";
import appSource from "./App.vue?raw";

describe("App project entry wiring", () => {
  it("opens directory picker instead of manual path form", () => {
    expect(appSource).toContain("ActiveProjectEntry");
    expect(appSource).toContain(":list-directories=\"store.listDirectories\"");
    expect(appSource).toContain("@select-directory=\"onDirectorySelected\"");
    expect(appSource).toContain("store.recordDirectoryHistory(path)");
    expect(appSource).not.toContain("Enter the absolute path to your project repository.");
  });

  it("contains terminal image paste wiring", () => {
    expect(appSource).toContain("@terminal-input=\"onTerminalInput\"");
    expect(appSource).toContain("@terminal-image-paste=\"onTerminalImagePaste\"");
  });

  it("marks task flag as read on task selection", () => {
    expect(appSource).toContain("async function onSelectTask(taskId: string)");
    expect(appSource).toContain("store.markTaskFlagReaded(taskId, true)");
  });

  it("submits SCM commit via store action", () => {
    expect(appSource).toContain("await store.submitTaskCommit(payload.taskId, payload.message)");
  });

  it("uses sonner toaster for error prompts", () => {
    expect(appSource).toContain("from \"@/components/ui/sonner\"");
    expect(appSource).toContain("<Toaster");
    expect(appSource).toContain("toast.error(");
  });

  it("wires remove project confirmation dialog", () => {
    expect(appSource).toContain("@remove-project=\"onRequestRemoveProject\"");
    expect(appSource).toContain("<AlertDialog v-model:open=\"showRemoveProjectDialog\">");
    expect(appSource).toContain("@click=\"onConfirmRemoveProject\"");
    expect(appSource).toContain("await store.removeActiveProject(projectId)");
  });

  it("wires archive-all-done action from project menu", () => {
    expect(appSource).toContain("@archive-project-done=\"onArchiveProjectDone\"");
    expect(appSource).toContain("await store.archiveDoneTasksByProject(id)");
  });

  it("wires helper openai settings fields", () => {
    expect(appSource).toContain(":helper-openai-endpoint=\"store.state.helperOpenAIEndpoint\"");
    expect(appSource).toContain(":helper-openai-model=\"store.state.helperOpenAIModel\"");
    expect(appSource).toContain(":helper-openai-api-key=\"store.state.helperOpenAIApiKey\"");
    expect(appSource).toContain("helperOpenAIApiKey: payload.helperOpenAIApiKey");
  });

  it("wires sidecar stop and restart context interactions", () => {
    expect(appSource).toContain("@stop-sidecar-chat=\"onStopSidecarChat\"");
    expect(appSource).toContain("@restart-sidecar-context=\"onRestartSidecarContext\"");
    expect(appSource).toContain("await store.stopTaskMessage(taskId)");
    expect(appSource).not.toContain("await store.setTaskSidecarMode(taskId, \"observer\")");
    expect(appSource).toContain("await store.createChildTask(taskId");
    expect(appSource).toContain("await store.createRootTask(projectId");
  });

  it("wires mobile thread send-message passthrough", () => {
    expect(appSource).toContain("@send-message=\"onSendTaskMessage\"");
  });
});
