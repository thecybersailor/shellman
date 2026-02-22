import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

type APIEnvelope<T> = {
  ok: boolean;
  data?: T;
  error?: { code?: string; message?: string };
};

const visitURL = process.env.E2E_VISIT_URL ?? "http://cli:4621";
const apiBaseURL = process.env.E2E_API_BASE ?? "http://cli:4621";
const e2eRepoRoot = process.env.E2E_REPO_ROOT ?? "/workspace";
const openAIKey = String(process.env.OPENAI_API_KEY ?? "").trim();

function hasUsableOpenAIKey() {
  if (openAIKey === "") return false;
  const lowered = openAIKey.toLowerCase();
  if (lowered.includes("xxxx")) return false;
  if (lowered.includes("placeholder")) return false;
  if (openAIKey.length < 20) return false;
  return true;
}

async function unwrap<T>(res: Awaited<ReturnType<APIRequestContext["get"]>> | Awaited<ReturnType<APIRequestContext["post"]>>) {
  if (!res.ok()) {
    const status = res.status();
    const text = await res.text();
    throw new Error(`unexpected http status=${status} body=${text}`);
  }
  const body = (await res.json()) as APIEnvelope<T>;
  expect(body.ok).toBe(true);
  return body.data as T;
}

async function waitForAPIReady(request: APIRequestContext, retries = 60) {
  for (let i = 0; i < retries; i += 1) {
    try {
      const res = await request.get(`${apiBaseURL}/healthz`);
      if (res.ok()) {
        return;
      }
    } catch {
      // retry
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`api is not ready: ${apiBaseURL}`);
}

async function seedProject(request: APIRequestContext) {
  await waitForAPIReady(request);
  const projectID = `e2e_docker_${Date.now()}`;
  const missingPaneTarget = `missing:${projectID}`;
  await unwrap<{ project_id: string }>(
    await request.post(`${apiBaseURL}/api/v1/projects/active`, {
      data: {
        project_id: projectID,
        repo_root: e2eRepoRoot
      }
    })
  );

  const rootTask = await unwrap<{ task_id: string; pane_target: string }>(
    await request.post(`${apiBaseURL}/api/v1/projects/${projectID}/panes/root`, {
      data: {
        title: "root-task"
      }
    })
  );

  const siblingTask = await unwrap<{ task_id: string; pane_target: string }>(
    await request.post(`${apiBaseURL}/api/v1/tasks/${rootTask.task_id}/panes/sibling`, {
      data: { title: "sibling-task" }
    })
  );

  const missingTask = await unwrap<{ task_id: string; pane_target: string }>(
    await request.post(`${apiBaseURL}/api/v1/tasks/${rootTask.task_id}/adopt-pane`, {
      data: {
        title: "missing-pane-task",
        pane_target: missingPaneTarget
      }
    })
  );

  return {
    projectID,
    rootTaskID: rootTask.task_id,
    siblingTaskID: siblingTask.task_id,
    missingTaskID: missingTask.task_id
  };
}

type TaskTreeNode = {
  task_id: string;
  current_command?: string;
  flag_readed?: boolean;
};

async function fetchProjectTree(request: APIRequestContext, projectID: string) {
  return unwrap<{ project_id: string; nodes: TaskTreeNode[] }>(
    await request.get(`${apiBaseURL}/api/v1/projects/${projectID}/tree`)
  );
}

async function waitForTaskNodes(
  request: APIRequestContext,
  projectID: string,
  taskIDs: string[],
  timeoutMs = 15000
) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const tree = await fetchProjectTree(request, projectID);
    const byID = new Map(tree.nodes.map((node) => [node.task_id, node]));
    const nodes: Record<string, TaskTreeNode> = {};
    let ready = true;
    for (const taskID of taskIDs) {
      const node = byID.get(taskID);
      if (!node) {
        ready = false;
        break;
      }
      nodes[taskID] = node;
    }
    if (ready) {
      return nodes;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`timeout waiting tasks from tree project=${projectID} taskIDs=${taskIDs.join(",")}`);
}

async function waitForTaskFlagReaded(
  request: APIRequestContext,
  projectID: string,
  taskID: string,
  expected: boolean,
  timeoutMs = 10000
) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const tree = await fetchProjectTree(request, projectID);
    const node = tree.nodes.find((item) => item.task_id === taskID);
    if (node && Boolean(node.flag_readed) === expected) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error(`timeout waiting task flag_readed=${String(expected)} project=${projectID} task=${taskID}`);
}

function projectRegion(page: Page, projectID: string) {
  return page.getByRole("region", { name: projectID });
}

function taskRowTitle(page: Page, projectID: string, taskID: string) {
  return projectRegion(page, projectID).getByTestId(`shellman-task-row-${taskID}`).getByTestId("shellman-task-row-title");
}

async function selectTask(page: Page, projectID: string, taskID: string) {
  const row = projectRegion(page, projectID).getByTestId(`shellman-task-row-${taskID}`).first();
  await expect(row).toBeVisible();
  await row.click();
  // Simulate human pacing to avoid panel transition races between task switches.
  await page.waitForTimeout(120);
}

function activeTerminal(page: Page) {
  return page.locator('[data-test-id="tt-terminal-root"]:visible').last();
}

function activeTerminalInput(page: Page) {
  return activeTerminal(page).getByTestId("tt-terminal-input").first();
}

async function runEcho(page: Page, token: string) {
  await activeTerminal(page).click();
  await page.keyboard.type(`echo ${token}`);
  await page.keyboard.press("Enter");
  await expect(activeTerminalInput(page)).not.toBeDisabled();
}

async function runCodexStyleRepaintStorm(page: Page) {
  const cmd = [
    "for i in $(seq 1 18); do",
    "printf '\\033[0m\\033[H\\033[2J';",
    "printf 'OpenAI Codex (v0.104.0)\\n';",
    "printf 'Find and fix a bug in @filename\\n';",
    "printf '? for shortcuts\\n';",
    "seq 1 90 | sed 's/^/codex-tui-line-/';",
    "sleep 0.05;",
    "done"
  ].join(" ");
  await activeTerminal(page).click();
  await page.keyboard.type(cmd);
  await page.keyboard.press("Enter");
}

async function countBufferLinesContaining(page: Page, needle: string) {
  return page.evaluate((text) => {
    const g = window as unknown as { __SHELLMAN_TERM_INSTANCES__?: Array<any> };
    const term = Array.isArray(g.__SHELLMAN_TERM_INSTANCES__) ? g.__SHELLMAN_TERM_INSTANCES__[g.__SHELLMAN_TERM_INSTANCES__.length - 1] : null;
    const active = term?.buffer?.active;
    if (!active || typeof active.length !== "number") {
      return -1;
    }
    let count = 0;
    for (let i = 0; i < active.length; i += 1) {
      const line = String(active.getLine(i)?.translateToString?.(true) ?? "");
      if (line.includes(text)) {
        count += 1;
      }
    }
    return count;
  }, needle);
}

async function setTaskSidecarMode(request: APIRequestContext, taskID: string, mode: "advisor" | "observer" | "autopilot") {
  await unwrap<{ task_id: string; sidecar_mode: string }>(
    await request.patch(`${apiBaseURL}/api/v1/tasks/${taskID}/sidecar-mode`, {
      data: { sidecar_mode: mode }
    })
  );
}

async function requestSelectPaneErrorCode(page: Page, target: string): Promise<string> {
  return page.evaluate(
    async (paneTarget) =>
      new Promise<string>((resolve, reject) => {
        const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
        const ws = new WebSocket(`${proto}//${window.location.host}/ws/client/local`);
        const reqID = `req_e2e_${Date.now()}`;
        const timer = window.setTimeout(() => {
          ws.close();
          reject(new Error("timeout waiting tmux.select_pane response"));
        }, 12000);

        ws.onerror = () => {
          window.clearTimeout(timer);
          reject(new Error("websocket error"));
        };
        ws.onopen = () => {
          ws.send(
            JSON.stringify({
              id: reqID,
              type: "req",
              op: "tmux.select_pane",
              payload: { target: paneTarget, cols: 80, rows: 24 }
            })
          );
        };
        ws.onmessage = (event) => {
          try {
            const msg = JSON.parse(String(event.data ?? "{}")) as {
              id?: string;
              type?: string;
              op?: string;
              error?: { code?: string };
            };
            if (msg.id !== reqID || msg.type !== "res" || msg.op !== "tmux.select_pane") {
              return;
            }
            window.clearTimeout(timer);
            ws.close();
            resolve(String(msg.error?.code ?? ""));
          } catch (error) {
            window.clearTimeout(timer);
            ws.close();
            reject(error instanceof Error ? error : new Error("invalid ws response"));
          }
        };
      }),
    target
  );
}

test.describe("shellman local web full chain (docker)", () => {
  test("health + first frame", async ({ page }) => {
    await page.goto(visitURL);
    await expect(page.locator("header")).toContainText("shellman");
    await expect(activeTerminal(page)).toBeVisible();
    await expect(activeTerminalInput(page)).toBeAttached();
  });

  test("settings keeps helper openai endpoint/model visible after save and refresh", async ({ page }) => {
    const endpoint = `https://openrouter.ai/api/v1/${Date.now()}`;
    const model = `openai/gpt-5-mini-${Date.now()}`;
    await page.goto(visitURL);

    await page.getByRole("button", { name: "Settings" }).first().click();
    await page.getByTestId("shellman-settings-helper-openai-endpoint").first().fill(endpoint);
    await page.getByTestId("shellman-settings-helper-openai-model").first().fill(model);
    await page.getByTestId("shellman-settings-save").first().click();
    await expect(page.getByTestId("shellman-settings-save").first()).toBeHidden();

    await page.waitForTimeout(1000);
    await page.getByRole("button", { name: "Settings" }).first().click();
    await expect(page.getByTestId("shellman-settings-helper-openai-endpoint").first()).toHaveValue(endpoint);
    await expect(page.getByTestId("shellman-settings-helper-openai-model").first()).toHaveValue(model);
    await page.keyboard.press("Escape");

    await page.waitForTimeout(1000);
    await page.reload();
    await page.getByRole("button", { name: "Settings" }).first().click();
    await expect(page.getByTestId("shellman-settings-helper-openai-endpoint").first()).toHaveValue(endpoint);
    await expect(page.getByTestId("shellman-settings-helper-openai-model").first()).toHaveValue(model);
  });

  test("live output and task switch still interactive", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.goto(visitURL);

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await runEcho(page, "__ROOT_LIVE__");

    await selectTask(page, seeded.projectID, seeded.siblingTaskID);
    await runEcho(page, "__SIBLING_LIVE__");

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await runEcho(page, "__ROOT_BACK__");
  });

  test("codex-style repaint stream keeps task switch viewport stable", async ({ page, request }) => {
    await page.addInitScript(() => {
      (window as unknown as { __SHELLMAN_TERM_DEBUG__?: boolean }).__SHELLMAN_TERM_DEBUG__ = true;
    });
    const seeded = await seedProject(request);
    await page.goto(visitURL);

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await runCodexStyleRepaintStorm(page);
    await page.waitForTimeout(320);

    // Human-paced switch interval to reproduce real usage.
    await selectTask(page, seeded.projectID, seeded.siblingTaskID);
    await page.waitForTimeout(420);
    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await page.waitForTimeout(520);

    await runEcho(page, "__CODEX_SWITCH_BACK_OK__");
    const duplicatedPromptLines = await countBufferLinesContaining(page, "Find and fix a bug in @filename");
    expect(duplicatedPromptLines).toBeGreaterThanOrEqual(0);
    expect(duplicatedPromptLines).toBeLessThanOrEqual(1);
  });

  test("task switch restores correct sidecar mode value", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await setTaskSidecarMode(request, seeded.rootTaskID, "observer");
    await setTaskSidecarMode(request, seeded.siblingTaskID, "autopilot");

    await page.goto(visitURL);

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await expect(page.getByTestId("shellman-shellman-sidecar-mode-trigger").first()).toContainText("Observer");

    await selectTask(page, seeded.projectID, seeded.siblingTaskID);
    await expect(page.getByTestId("shellman-shellman-sidecar-mode-trigger").first()).toContainText("Autopilot");

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await expect(page.getByTestId("shellman-shellman-sidecar-mode-trigger").first()).toContainText("Observer");
  });

  test("refresh page keeps terminal usable", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.goto(visitURL);

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await runEcho(page, "__BEFORE_REFRESH__");

    await page.reload();
    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await runEcho(page, "__AFTER_REFRESH__");
  });

  test("marks flag_readed after selecting task", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.goto(visitURL);

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await waitForTaskFlagReaded(request, seeded.projectID, seeded.rootTaskID, true);
  });

  test("reload first frame keeps task rows available in sidebar", async ({ page, request }) => {
    test.setTimeout(60_000);
    const projectID = `e2e_first_frame_cmd_${Date.now()}`;
    await unwrap<{ project_id: string }>(
      await request.post(`${apiBaseURL}/api/v1/projects/active`, {
        data: {
          project_id: projectID,
          repo_root: e2eRepoRoot
        }
      })
    );
    const rootTask = await unwrap<{ task_id: string }>(
      await request.post(`${apiBaseURL}/api/v1/projects/${projectID}/panes/root`, {
        data: {
          title: ""
        }
      })
    );
    const siblingTask = await unwrap<{ task_id: string }>(
      await request.post(`${apiBaseURL}/api/v1/tasks/${rootTask.task_id}/panes/sibling`, {
        data: { title: "" }
      })
    );

    await page.goto(visitURL);
    await selectTask(page, projectID, rootTask.task_id);
    await runEcho(page, "__FIRST_FRAME_ROOT__");
    await waitForTaskNodes(request, projectID, [rootTask.task_id], 30000);
    await selectTask(page, projectID, siblingTask.task_id);
    await runEcho(page, "__FIRST_FRAME_SIBLING__");
    await waitForTaskNodes(request, projectID, [siblingTask.task_id], 30000);

    const persisted = await waitForTaskNodes(request, projectID, [rootTask.task_id, siblingTask.task_id], 45000);
    await page.reload();

    expect(persisted[rootTask.task_id]?.task_id).toBe(rootTask.task_id);
    expect(persisted[siblingTask.task_id]?.task_id).toBe(siblingTask.task_id);
    await expect(taskRowTitle(page, projectID, rootTask.task_id)).toBeVisible();
    await expect(taskRowTitle(page, projectID, siblingTask.task_id)).toBeVisible();
  });

  test("missing pane is deactive and reopen available", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.goto(visitURL);

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await runEcho(page, "__SNAPSHOT_BASE__");

    await selectTask(page, seeded.projectID, seeded.missingTaskID);

    const reopenButton = page.getByTestId("shellman-reopen-pane-button").first();
    await expect(reopenButton).toBeVisible({ timeout: 15000 });
    await expect(reopenButton).toBeEnabled();

    const input = activeTerminalInput(page);
    await expect(input).toBeDisabled();
    await reopenButton.click();

    await expect(input).not.toBeDisabled();
    await runEcho(page, "__REOPENED__");
  });

  test("tmux.select_pane returns TMUX_PANE_NOT_FOUND for missing target", async ({ page }) => {
    await page.goto(visitURL);
    const code = await requestSelectPaneErrorCode(page, `missing:e2e_${Date.now()}`);
    expect(code).toBe("TMUX_PANE_NOT_FOUND");
  });

  test("shellman chat sends user message and receives assistant reply", async ({ page, request }) => {
    test.skip(!hasUsableOpenAIKey(), "requires usable OPENAI_API_KEY for live assistant assertion");
    const seeded = await seedProject(request);
    await page.goto(visitURL);
    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await expect(page.getByTestId("shellman-task-title-input")).toHaveValue("root-task");
    await unwrap(
      await request.post(`${apiBaseURL}/api/v1/tasks/${seeded.rootTaskID}/messages`, {
        data: { content: "Reply exactly: SHELLMAN_E2E_OK" }
      })
    );
    await page.reload();
    await selectTask(page, seeded.projectID, seeded.rootTaskID);

    await expect(page.getByTestId("shellman-shellman-message-user").last()).toContainText("SHELLMAN_E2E_OK", { timeout: 15000 });
    await expect(page.getByText("SHELLMAN_E2E_OK").last()).toBeVisible({ timeout: 15000 });

    await expect
      .poll(
        async () => {
          const res = await request.get(`${apiBaseURL}/api/v1/tasks/${seeded.rootTaskID}/messages`);
          if (!res.ok()) return "http_error";
          const body = await res.json();
          const messages = Array.isArray(body?.data?.messages) ? body.data.messages : [];
          const assistant = [...messages].reverse().find((m: any) => m?.role === "assistant");
          if (!assistant) return "assistant_missing";
          const status = String(assistant.status ?? "");
          const content = String(assistant.content ?? "").trim();
          return `${status}:${content.length}`;
        },
        { timeout: 60000, intervals: [500, 1000, 1500] }
      )
      .toMatch(/^completed:[1-9]\d*$/);
  });

  test("shellman chat timeline keeps earlier turns after second send", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.goto(visitURL);
    await selectTask(page, seeded.projectID, seeded.rootTaskID);

    await unwrap(
      await request.post(`${apiBaseURL}/api/v1/tasks/${seeded.rootTaskID}/messages`, {
        data: { content: "TURN_ONE_MARKER" }
      })
    );
    await unwrap(
      await request.post(`${apiBaseURL}/api/v1/tasks/${seeded.rootTaskID}/messages`, {
        data: { content: "TURN_TWO_MARKER" }
      })
    );

    await page.reload();
    await selectTask(page, seeded.projectID, seeded.rootTaskID);

    await expect(page.getByTestId("shellman-shellman-message-user").filter({ hasText: "TURN_ONE_MARKER" }).first()).toBeVisible({
      timeout: 20000
    });
    await expect(page.getByTestId("shellman-shellman-message-user").filter({ hasText: "TURN_TWO_MARKER" }).first()).toBeVisible({
      timeout: 20000
    });

    await expect
      .poll(
        async () => {
          const res = await request.get(`${apiBaseURL}/api/v1/tasks/${seeded.rootTaskID}/messages`);
          if (!res.ok()) return 0;
          const body = await res.json();
          const messages = Array.isArray(body?.data?.messages) ? body.data.messages : [];
          return messages.filter((m: any) => m?.role === "user").length;
        },
        { timeout: 30000, intervals: [500, 1000, 1500] }
      )
      .toBeGreaterThanOrEqual(2);
  });

  test("shellman renders ai-elements tool block from structured assistant content", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.route(`**/api/v1/tasks/${seeded.rootTaskID}/messages`, async (route) => {
      if (route.request().method() !== "GET") {
        await route.fallback();
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          ok: true,
          data: {
            task_id: seeded.rootTaskID,
            messages: [
              {
                id: 101,
                task_id: seeded.rootTaskID,
                role: "user",
                content: "tool test",
                status: "completed",
                created_at: 1,
                updated_at: 1
              },
              {
                id: 102,
                task_id: seeded.rootTaskID,
                role: "assistant",
                status: "completed",
                content: JSON.stringify({
                  text: "tool finished",
                  tools: [
                    {
                      type: "dynamic-tool",
                      state: "output-available",
                      tool_name: "gateway_http",
                      input: { method: "GET", path: "/healthz" },
                      output: { status: 200, body: "{\"status\":\"ok\"}" }
                    }
                  ]
                }),
                created_at: 2,
                updated_at: 2
              }
            ]
          }
        })
      });
    });

    await page.goto(visitURL);
    await selectTask(page, seeded.projectID, seeded.rootTaskID);

    await expect(page.getByText("tool test").first()).toBeVisible();
    await expect(page.getByText("tool finished").first()).toBeVisible();
    await expect(page.getByTestId("shellman-shellman-tool").first()).toBeVisible();
    await expect(page.getByTestId("shellman-shellman-tool-header").first()).toContainText("gateway_http");
    const toolHeader = page.getByTestId("shellman-shellman-tool-header").first();
    await toolHeader.focus();
    await toolHeader.press("Enter");
    await expect(page.getByTestId("shellman-shellman-tool-content").first()).toBeVisible();
    await expect(page.getByTestId("shellman-shellman-tool-input").first()).toBeVisible();
    await expect(page.getByTestId("shellman-shellman-tool-output").first()).toBeVisible();
  });

  test("shellman renders responding indicator for running assistant message", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.route(`**/api/v1/tasks/${seeded.rootTaskID}/messages`, async (route) => {
      if (route.request().method() !== "GET") {
        await route.fallback();
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          ok: true,
          data: {
            task_id: seeded.rootTaskID,
            messages: [
              {
                id: 201,
                task_id: seeded.rootTaskID,
                role: "assistant",
                content: "",
                status: "running",
                created_at: 1,
                updated_at: 1
              }
            ]
          }
        })
      });
    });

    await page.goto(visitURL);
    await selectTask(page, seeded.projectID, seeded.rootTaskID);

    await expect(page.getByTestId("shellman-shellman-message-assistant").first()).toBeVisible();
    await expect(page.getByTestId("shellman-shellman-responding").first()).toBeVisible();
  });

  test("shellman chat ui updates before /messages response returns", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.goto(visitURL);
    await selectTask(page, seeded.projectID, seeded.rootTaskID);

    let requestObserved = false;
    let responseReturned = false;
    await page.route(`**/api/v1/tasks/${seeded.rootTaskID}/messages`, async (route) => {
      if (route.request().method() !== "POST") {
        await route.fallback();
        return;
      }
      requestObserved = true;
      const responsePromise = route.fetch();
      await page.waitForTimeout(4000);
      const response = await responsePromise;
      responseReturned = true;
      await route.fulfill({ response });
    });

    await page.getByTestId("shellman-shellman-input").fill("Reply exactly: SHELLMAN_E2E_TIMING");
    await page.getByTestId("shellman-shellman-send").click();

    await expect.poll(() => requestObserved).toBe(true);
    await expect.poll(() => page.getByTestId("shellman-shellman-message-user").count(), { timeout: 3500 }).toBeGreaterThan(0);
    await expect.poll(() => page.getByTestId("shellman-shellman-message-assistant").count(), { timeout: 3500 }).toBeGreaterThan(0);
    await expect(page.getByTestId("shellman-shellman-responding").last()).toBeVisible();
    expect(responseReturned).toBe(false);

    await expect.poll(() => responseReturned, { timeout: 30000 }).toBe(true);
    await expect(page.getByTestId("shellman-shellman-message-user").last()).toContainText("SHELLMAN_E2E_TIMING", { timeout: 30000 });
    await expect(page.getByTestId("shellman-shellman-message-assistant").last()).toBeVisible({ timeout: 30000 });
  });
});
