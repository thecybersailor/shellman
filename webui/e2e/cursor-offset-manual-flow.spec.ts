import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

type APIEnvelope<T> = {
  ok: boolean;
  data?: T;
};

const visitURL = process.env.E2E_VISIT_URL ?? "http://127.0.0.1:4621";
const apiBaseURL = process.env.E2E_API_BASE ?? "http://127.0.0.1:4621";
const e2eRepoRoot = process.env.E2E_REPO_ROOT ?? "/workspace";

async function unwrap<T>(res: Awaited<ReturnType<APIRequestContext["get"]>> | Awaited<ReturnType<APIRequestContext["post"]>>) {
  expect(res.ok()).toBe(true);
  const body = (await res.json()) as APIEnvelope<T>;
  expect(body.ok).toBe(true);
  return body.data as T;
}

async function seedProject(request: APIRequestContext) {
  const projectID = `e2e_manual_${Date.now()}`;
  await unwrap<{ project_id: string }>(
    await request.post(`${apiBaseURL}/api/v1/projects/active`, {
      data: { project_id: projectID, repo_root: e2eRepoRoot }
    })
  );
  const rootTask = await unwrap<{ task_id: string }>(
    await request.post(`${apiBaseURL}/api/v1/projects/${projectID}/panes/root`, { data: { title: "root-task" } })
  );
  const siblingTask = await unwrap<{ task_id: string }>(
    await request.post(`${apiBaseURL}/api/v1/tasks/${rootTask.task_id}/panes/sibling`, { data: { title: "sibling-task" } })
  );
  return { projectID, rootTaskID: rootTask.task_id, siblingTaskID: siblingTask.task_id };
}

function projectRegion(page: Page, projectID: string) {
  return page.getByRole("region", { name: projectID });
}

async function selectTask(page: Page, projectID: string, taskID: string) {
  await projectRegion(page, projectID).getByTestId(`shellman-task-row-${taskID}`).first().click();
}

function activeTerminal(page: Page) {
  return page.locator('[data-test-id="tt-terminal-root"]:visible').last();
}

async function expectTerminalFocused(page: Page) {
  await expect
    .poll(async () =>
      page.evaluate(() => {
        const root = Array.from(document.querySelectorAll('[data-test-id="tt-terminal-root"]')).find(
          (el) => (el as HTMLElement).offsetParent !== null
        ) as HTMLElement | undefined;
        if (!root) {
          return false;
        }
        const input = root.querySelector("textarea.xterm-helper-textarea") as HTMLTextAreaElement | null;
        return Boolean(input && document.activeElement === input);
      })
    )
    .toBe(true);
}

async function readState(page: Page) {
  return page.evaluate(() => {
    const g = window as unknown as { __SHELLMAN_TERM_INSTANCES__?: Array<any>; __SHELLMAN_TERM_DEBUG_LOGS__?: Array<any> };
    const term = Array.isArray(g.__SHELLMAN_TERM_INSTANCES__) ? g.__SHELLMAN_TERM_INSTANCES__[g.__SHELLMAN_TERM_INSTANCES__.length - 1] : null;
    const active = term?.buffer?.active ?? null;
    if (!active) {
      return { ok: false, reason: "no-active-buffer" };
    }
    const lines: string[] = [];
    for (let i = 0; i < active.length; i += 1) {
      lines.push(String(active.getLine(i)?.translateToString?.(true) ?? ""));
    }
    const promptLineIndex = lines.reduce(
      (acc, line, idx) => (line.includes("/workspace/cli#") ? idx : acc),
      -1
    );
    const debugTail = (g.__SHELLMAN_TERM_DEBUG_LOGS__ ?? []).slice(-40);
    const canvasCursor = (() => {
      const root = Array.from(document.querySelectorAll('[data-test-id="tt-terminal-root"]')).find((el) =>
        (el as HTMLElement).offsetParent !== null
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
      return { found: true, selected: candidates[0], candidates };
    })();
    const renderDims = term?._core?._renderService?.dimensions?.css ?? null;
    const cellHeight = typeof renderDims?.cell?.height === "number" ? renderDims.cell.height : null;
    const visualCursorRow =
      canvasCursor && (canvasCursor as any).found && cellHeight && cellHeight > 0
        ? Math.round(((canvasCursor as any).selected.cssTop as number) / cellHeight)
        : null;

    return {
      ok: true,
      cursorX: active.cursorX,
      cursorY: active.cursorY,
      viewportY: active.viewportY,
      baseY: active.baseY,
      length: active.length,
      promptLineIndex,
      cursorPromptDelta: promptLineIndex >= 0 ? active.cursorY - promptLineIndex : null,
      cellHeight,
      visualCursorRow,
      visualCursorDelta: visualCursorRow !== null ? visualCursorRow - active.cursorY : null,
      canvasCursor,
      tail: lines.slice(Math.max(0, promptLineIndex - 3), promptLineIndex + 4),
      debugTail
    };
  });
}

async function step(page: Page, index: number, name: string, action: () => Promise<void>) {
  await action();
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `../logs/manual-flow/${String(index).padStart(2, "0")}-${name}.png`, fullPage: true });
}

test("manual cursor flow with 1s interval", async ({ page, request }) => {
  await page.addInitScript(() => {
    (window as unknown as { __SHELLMAN_TERM_DEBUG__?: boolean }).__SHELLMAN_TERM_DEBUG__ = true;
  });

  const seeded = await seedProject(request);
  await page.goto(visitURL);
  await expect(activeTerminal(page)).toBeVisible();

  let i = 1;
  await step(page, i++, "open-task-1", async () => {
    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await expectTerminalFocused(page);
  });

  await step(page, i++, "task-1-enter-shell", async () => {
    await activeTerminal(page).click();
  });

  await step(page, i++, "task-1-type-l", async () => {
    await page.keyboard.type("l");
  });

  await step(page, i++, "task-1-type-s", async () => {
    await page.keyboard.type("s");
  });

  await step(page, i++, "task-1-enter", async () => {
    await page.keyboard.press("Enter");
  });

  await step(page, i++, "open-task-2", async () => {
    await selectTask(page, seeded.projectID, seeded.siblingTaskID);
    await expectTerminalFocused(page);
  });

  await step(page, i++, "task-2-type-p", async () => {
    await page.keyboard.type("p");
  });

  await step(page, i++, "task-2-type-w", async () => {
    await page.keyboard.type("w");
  });

  await step(page, i++, "task-2-type-d", async () => {
    await page.keyboard.type("d");
  });

  await step(page, i++, "task-2-enter", async () => {
    await page.keyboard.press("Enter");
  });

  await step(page, i++, "switch-back-task-1", async () => {
    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await expectTerminalFocused(page);
  });

  await step(page, i++, "switch-back-task-1-idle-2s", async () => {
    await page.waitForTimeout(2000);
  });

  await step(page, i++, "switch-back-task-1-run-ls", async () => {
    await page.keyboard.type("ls");
    await page.keyboard.press("Enter");
  });

  const state = await readState(page);
  // eslint-disable-next-line no-console
  console.log(`[manual-flow-state] ${JSON.stringify(state)}`);
});
