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
  await expect(row).toBeVisible({ timeout: 30000 });
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

async function sendTTYInput(request: APIRequestContext, taskID: string, input: string) {
  await unwrap(
    await request.post(`${apiBaseURL}/api/v1/tasks/${taskID}/messages`, {
      data: {
        source: "tty_write_stdin",
        input
      }
    })
  );
}

async function runMockCodexTUI(request: APIRequestContext, taskID: string, paneToken: string, lines: number) {
  const cmd = `bash /workspace/scripts/e2e/codex_tui_mock.sh ${paneToken} ${String(lines)}\r`;
  await sendTTYInput(request, taskID, cmd);
}

async function startCodexMockCommand(request: APIRequestContext, taskID: string, paneToken: string, lines: number) {
  const cmd = `CODEX_MOCK_TOKEN=${paneToken} CODEX_MOCK_LINES=${String(lines)} codex\r`;
  await sendTTYInput(request, taskID, cmd);
}

async function waitBufferContains(page: Page, text: string, timeoutMs = 12000) {
  await expect
    .poll(
      async () =>
        page.evaluate((needle) => {
          const g = window as unknown as { __SHELLMAN_TERM_INSTANCES__?: Array<any> };
          const terms = Array.isArray(g.__SHELLMAN_TERM_INSTANCES__) ? g.__SHELLMAN_TERM_INSTANCES__ : [];
          const term =
            terms.findLast?.((item) => Boolean(item?.element && item.element.offsetParent !== null)) ??
            terms[terms.length - 1] ??
            null;
          const active = term?.buffer?.active;
          if (!active || typeof active.length !== "number") {
            return false;
          }
          for (let i = 0; i < active.length; i += 1) {
            const line = String(active.getLine(i)?.translateToString?.(true) ?? "");
            if (line.includes(needle)) {
              return true;
            }
          }
          return false;
        }, text),
      { timeout: timeoutMs }
    )
    .toBe(true);
}

async function bufferContains(page: Page, text: string) {
  return page.evaluate((needle) => {
    const g = window as unknown as { __SHELLMAN_TERM_INSTANCES__?: Array<any> };
    const terms = Array.isArray(g.__SHELLMAN_TERM_INSTANCES__) ? g.__SHELLMAN_TERM_INSTANCES__ : [];
    const term =
      terms.findLast?.((item) => Boolean(item?.element && item.element.offsetParent !== null)) ??
      terms[terms.length - 1] ??
      null;
    const active = term?.buffer?.active;
    if (!active || typeof active.length !== "number") {
      return false;
    }
    for (let i = 0; i < active.length; i += 1) {
      const line = String(active.getLine(i)?.translateToString?.(true) ?? "");
      if (line.includes(needle)) {
        return true;
      }
    }
    return false;
  }, text);
}

async function readPaneHistoryFetchURLs(page: Page) {
  return page.evaluate(() => {
    const g = window as unknown as { __SHELLMAN_FETCH_URLS__?: string[] };
    return Array.isArray(g.__SHELLMAN_FETCH_URLS__) ? g.__SHELLMAN_FETCH_URLS__.slice() : [];
  });
}

async function readSelectPanePayloads(page: Page) {
  return page.evaluate(() => {
    const g = window as unknown as { __SHELLMAN_SELECT_PAYLOADS__?: Array<Record<string, unknown>> };
    return Array.isArray(g.__SHELLMAN_SELECT_PAYLOADS__) ? g.__SHELLMAN_SELECT_PAYLOADS__.slice() : [];
  });
}

async function readCursorAlignment(page: Page) {
  return page.evaluate(() => {
    const g = window as unknown as { __SHELLMAN_TERM_INSTANCES__?: Array<any>; __SHELLMAN_TERM_DEBUG_LOGS__?: Array<any> };
    const terms = Array.isArray(g.__SHELLMAN_TERM_INSTANCES__) ? g.__SHELLMAN_TERM_INSTANCES__ : [];
    const term =
      terms.findLast?.((item) => Boolean(item?.element && item.element.offsetParent !== null)) ??
      terms[terms.length - 1] ??
      null;
    const active = term?.buffer?.active ?? null;
    if (!active) {
      return { ok: false, reason: "no-active-buffer" };
    }
    const lines: string[] = [];
    for (let i = 0; i < active.length; i += 1) {
      lines.push(String(active.getLine(i)?.translateToString?.(true) ?? ""));
    }
    const promptLineIndex = lines.reduce((acc, line, idx) => (line.includes("root@") || line.includes("#") ? idx : acc), -1);
    const canvasCursor = (() => {
      const root = Array.from(document.querySelectorAll('[data-test-id="tt-terminal-root"]')).find(
        (el) => (el as HTMLElement).offsetParent !== null
      ) as HTMLElement | undefined;
      if (!root) {
        return { found: false, reason: "no-terminal-root" };
      }
      const canvases = Array.from(root.querySelectorAll(".xterm-screen canvas")) as HTMLCanvasElement[];
      const candidates: Array<Record<string, number>> = [];
      for (let idx = 0; idx < canvases.length; idx += 1) {
        const c = canvases[idx];
        const ctx = c.getContext("2d");
        if (!ctx || c.width <= 0 || c.height <= 0) {
          continue;
        }
        const data = ctx.getImageData(0, 0, c.width, c.height).data;
        let minX = c.width;
        let minY = c.height;
        let maxX = -1;
        let maxY = -1;
        let count = 0;
        for (let i = 0; i < data.length; i += 4) {
          const r = data[i];
          const gg = data[i + 1];
          const b = data[i + 2];
          const a = data[i + 3];
          if (a < 210 || r < 230 || gg < 230 || b < 230) {
            continue;
          }
          const p = i / 4;
          const x = p % c.width;
          const y = Math.floor(p / c.width);
          count += 1;
          if (x < minX) minX = x;
          if (y < minY) minY = y;
          if (x > maxX) maxX = x;
          if (y > maxY) maxY = y;
        }
        if (count < 20 || maxX < minX || maxY < minY) {
          continue;
        }
        const rect = c.getBoundingClientRect();
        const dpr = rect.width > 0 ? c.width / rect.width : 1;
        candidates.push({
          index: idx,
          count,
          minX,
          minY,
          maxX,
          maxY,
          cssTop: minY / dpr,
          cssHeight: (maxY - minY + 1) / dpr
        });
      }
      if (candidates.length === 0) {
        return { found: false, reason: "no-cursor-candidate", canvasCount: canvases.length };
      }
      candidates.sort((a, b) => (a.cssHeight as number) - (b.cssHeight as number));
      return { found: true, selected: candidates[0] };
    })();
    const renderDims = term?._core?._renderService?.dimensions?.css ?? null;
    const cellHeight = typeof renderDims?.cell?.height === "number" ? renderDims.cell.height : null;
    const viewportY = typeof active.viewportY === "number" ? active.viewportY : 0;
    const promptViewportRow = promptLineIndex >= 0 ? promptLineIndex - viewportY : null;
    const visualCursorRow =
      canvasCursor && (canvasCursor as any).found && cellHeight && cellHeight > 0
        ? Math.round(((canvasCursor as any).selected.cssTop as number) / cellHeight)
        : null;
    return {
      ok: true,
      cursorY: active.cursorY,
      promptLineIndex,
      promptViewportRow,
      cursorPromptDelta: promptViewportRow !== null ? active.cursorY - promptViewportRow : null,
      visualCursorRow,
      visualCursorDelta: visualCursorRow !== null ? visualCursorRow - active.cursorY : null,
      debugTail: (g.__SHELLMAN_TERM_DEBUG_LOGS__ ?? []).slice(-20)
    };
  });
}

async function seedProjectWithManyRootTasks(request: APIRequestContext, count: number) {
  if (count < 2) {
    throw new Error(`invalid task count: ${String(count)}`);
  }
  await waitForAPIReady(request);
  const projectID = `e2e_docker_lru_${Date.now()}`;
  await unwrap<{ project_id: string }>(
    await request.post(`${apiBaseURL}/api/v1/projects/active`, {
      data: {
        project_id: projectID,
        repo_root: e2eRepoRoot
      }
    })
  );
  const entries: Array<{ projectID: string; taskID: string; paneTarget: string }> = [];
  for (let i = 0; i < count; i += 1) {
    const rootTask = await unwrap<{ task_id: string; pane_target: string }>(
      await request.post(`${apiBaseURL}/api/v1/projects/${projectID}/panes/root`, {
        data: {
          title: `root-task-${String(i + 1).padStart(2, "0")}`
        }
      })
    );
    entries.push({
      projectID,
      taskID: rootTask.task_id,
      paneTarget: String(rootTask.pane_target ?? "").trim()
    });
  }
  return entries;
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

  test("10 panes with 5000 lines should evict LRU and recover evicted panes with gap_recover", async ({ page, request }) => {
    test.setTimeout(240_000);
    await page.addInitScript(() => {
      const g = window as unknown as {
        __SHELLMAN_TERM_DEBUG__?: boolean;
        __SHELLMAN_FETCH_URLS__?: string[];
        __SHELLMAN_SELECT_PAYLOADS__?: Array<Record<string, unknown>>;
        fetch?: typeof window.fetch;
      };
      g.__SHELLMAN_TERM_DEBUG__ = true;
      g.__SHELLMAN_FETCH_URLS__ = [];
      g.__SHELLMAN_SELECT_PAYLOADS__ = [];
      const originFetch = window.fetch.bind(window);
      window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : String(input.url ?? "");
        if (url.includes("/api/v1/tasks/") && url.includes("/pane-history")) {
          g.__SHELLMAN_FETCH_URLS__?.push(url);
        }
        return originFetch(input, init);
      };
      const originSend = WebSocket.prototype.send;
      WebSocket.prototype.send = function (data: string | ArrayBufferLike | Blob | ArrayBufferView) {
        if (typeof data === "string") {
          try {
            const parsed = JSON.parse(data) as { op?: string; payload?: Record<string, unknown> };
            if (parsed.op === "tmux.select_pane" && parsed.payload && typeof parsed.payload === "object") {
              g.__SHELLMAN_SELECT_PAYLOADS__?.push({ ...parsed.payload });
            }
          } catch {
            // ignore parse failures
          }
        }
        return originSend.apply(this, [data as any]);
      };
    });

    const seeded = await seedProjectWithManyRootTasks(request, 10);
    await page.goto(visitURL);

    for (let i = 0; i < seeded.length; i += 1) {
      const taskID = seeded[i].taskID;
      const projectID = seeded[i].projectID;
      const paneToken = `T${String(i + 1).padStart(2, "0")}`;
      await selectTask(page, projectID, taskID);
      if (i === 0) {
        await startCodexMockCommand(request, taskID, paneToken, 5000);
        await waitBufferContains(page, "Find and fix a bug in @filename", 20000);
      } else {
        await runMockCodexTUI(request, taskID, paneToken, 5000);
        await waitBufferContains(page, `PANE_${paneToken}_LINE_05000`, 20000);
      }
      await page.waitForTimeout(180);
    }

    const evicted = seeded.slice(0, 5);
    for (let i = 0; i < evicted.length; i += 1) {
      const taskID = evicted[i].taskID;
      const projectID = evicted[i].projectID;
      const paneToken = `T${String(i + 1).padStart(2, "0")}`;
      await selectTask(page, projectID, taskID);
      await page.waitForTimeout(1200);
      await activeTerminal(page).click();
      if (i === 0 || i === evicted.length - 1) {
        if (i === 0) {
          await waitBufferContains(page, "Find and fix a bug in @filename", 45000);
        } else {
          await waitBufferContains(page, `PANE_${paneToken}_LINE_05000`, 45000);
          expect(await bufferContains(page, `PANE_${paneToken}_LINE_00001`)).toBe(false);
        }
      }
      await expect
        .poll(async () => {
          const alignment = await readCursorAlignment(page);
          return alignment.ok ? alignment.visualCursorDelta : null;
        })
        .toBe(0);
      const alignmentByBuffer = await readCursorAlignment(page);
      // eslint-disable-next-line no-console
      console.log(`[lru-gap-recover][buffer] i=${i} token=${paneToken} state=${JSON.stringify(alignmentByBuffer)}`);
      expect(alignmentByBuffer.ok).toBe(true);
      if (alignmentByBuffer.promptLineIndex !== -1) {
        expect(alignmentByBuffer.cursorPromptDelta).toBe(0);
      }
    }

    for (let round = 0; round < 2; round += 1) {
      for (let i = 0; i < evicted.length; i += 1) {
        const taskID = evicted[i].taskID;
        const projectID = evicted[i].projectID;
        await selectTask(page, projectID, taskID);
        await page.waitForTimeout(360);
        await activeTerminal(page).click();
        const alignment = await readCursorAlignment(page);
        expect(alignment.ok).toBe(true);
        expect(alignment.visualCursorDelta).toBe(0);
        // eslint-disable-next-line no-console
        console.log(`[lru-gap-recover][round] round=${round} i=${i} state=${JSON.stringify(alignment)}`);
        if (alignment.promptLineIndex !== -1) {
          expect(alignment.cursorPromptDelta).toBe(0);
        }
      }
    }

    const selectPayloads = await readSelectPanePayloads(page);
    for (const item of evicted) {
      expect(
        selectPayloads.some(
          (payload) =>
            String(payload.target ?? "") === item.paneTarget &&
            payload.gap_recover === true &&
            Number(payload.history_lines ?? 0) >= 4000
        )
      ).toBe(true);
    }

    // Optional signal: manual history pull API should still be idle in this flow (no top-scroll pull).
    const paneHistoryURLs = await readPaneHistoryFetchURLs(page);
    expect(paneHistoryURLs.length).toBe(0);
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

  test("shellman switches send to stop when running assistant and empty input, then calls messages/stop", async ({ page, request }) => {
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
                id: 301,
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

    let stopCalled = false;
    await page.route(`**/api/v1/tasks/${seeded.rootTaskID}/messages/stop`, async (route) => {
      if (route.request().method() !== "POST") {
        await route.fallback();
        return;
      }
      stopCalled = true;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          ok: true,
          data: {
            task_id: seeded.rootTaskID,
            canceled: true
          }
        })
      });
    });

    await page.goto(visitURL);
    await selectTask(page, seeded.projectID, seeded.rootTaskID);

    await expect(page.getByTestId("shellman-shellman-stop")).toBeVisible();
    await expect(page.getByTestId("shellman-shellman-send")).toHaveCount(0);
    await page.getByTestId("shellman-shellman-stop").click();
    await expect.poll(() => stopCalled).toBe(true);

    await page.getByTestId("shellman-shellman-input").fill("continue");
    await expect(page.getByTestId("shellman-shellman-send")).toBeVisible();
    await expect(page.getByTestId("shellman-shellman-stop")).toHaveCount(0);
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
