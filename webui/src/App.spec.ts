import { describe, expect, it } from "vitest";
import appSource from "./App.vue?raw";

describe("App project entry wiring", () => {
  it("opens directory picker instead of manual path form", () => {
    expect(appSource).toContain("ActiveProjectEntry");
    expect(appSource).toContain(":get-f-s-roots=\"store.getFSRoots\"");
    expect(appSource).toContain(":list-directories=\"store.listDirectories\"");
    expect(appSource).toContain("@select-directory=\"onDirectorySelected\"");
    expect(appSource).toContain("store.recordDirectoryHistory(path)");
    expect(appSource).not.toContain("Enter the absolute path to your project repository.");
  });

  it("contains terminal image paste wiring", () => {
    expect(appSource).toContain("@terminal-input=\"onTerminalInput\"");
    expect(appSource).toContain("@terminal-image-paste=\"onTerminalImagePaste\"");
  });

  it("wires terminal-link-open from terminal panes", () => {
    expect(appSource).toContain("@terminal-link-open=\"onTerminalLinkOpen\"");
  });

  it("guards path links by selected project root", () => {
    expect(appSource).toContain("resolvePathLinkInProject(payload.raw, projectRoot)");
    expect(appSource).toContain("selectedTaskProjectRoot.value");
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

  it("wires edit project display name action", () => {
    expect(appSource).toContain("@edit-project=\"onRequestEditProjectName\"");
    expect(appSource).toContain("<AlertDialog v-model:open=\"showEditProjectDialog\">");
    expect(appSource).toContain("await store.renameProjectDisplayName(projectId, nextDisplayName)");
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

  it("wires running assistant stop interaction from thread panel", () => {
    expect(appSource).toContain("@stop-running-assistant-message=\"onStopRunningAssistantMessage\"");
    expect(appSource).toContain("await store.stopTaskMessage(taskId)");
  });

  it("wires mobile thread send-message passthrough", () => {
    expect(appSource).toContain("@send-message=\"onSendTaskMessage\"");
  });

  it("wires file editor header actions for close and save", () => {
    expect(appSource).toContain("data-test-id=\"shellman-file-viewer-close\"");
    expect(appSource).toContain("data-test-id=\"shellman-file-viewer-save\"");
    expect(appSource).toContain("@click=\"onCloseFileViewer\"");
    expect(appSource).toContain("@click=\"onSaveFileViewer\"");
    expect(appSource).toContain("async function onSaveFileViewer() {");
  });

  it("wires overview sheet open state and event bridge", () => {
    expect(appSource).toContain("const showOverviewSheet = ref(false)");
    expect(appSource).toContain("@open-overview=\"onOpenOverview('desktop')\"");
    expect(appSource).toContain("@open-overview=\"onOpenOverview('mobile')\"");
    expect(appSource).toContain("<OverviewSheet");
    expect(appSource).toContain("v-model:open=\"showOverviewSheet\"");
    expect(appSource).toContain(":projects=\"projects\"");
    expect(appSource).toContain("@select-task=\"onSelectTask\"");
    expect(appSource).toContain(":pm-sessions=\"overviewPmSessions\"");
    expect(appSource).toContain(":selected-pm-session-id=\"overviewPmSessionId\"");
    expect(appSource).toContain(":pm-messages=\"overviewPmMessages\"");
    expect(appSource).toContain("@send-pm-message=\"onOverviewSendPMMessage\"");
  });
});
