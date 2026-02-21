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
  const projectID = `e2e_cursor_${Date.now()}`;
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
  await projectRegion(page, projectID).getByTestId(`muxt-task-row-${taskID}`).first().click();
}

function activeTerminal(page: Page) {
  return page.locator('[data-test-id="tt-terminal-root"]:visible').last();
}

async function runCommand(page: Page, command: string) {
  await activeTerminal(page).click();
  await page.keyboard.type(command);
  await page.keyboard.press("Enter");
}

async function runCommands(page: Page, commands: string[]) {
  for (const cmd of commands) {
    await runCommand(page, cmd);
  }
}

async function readCursorState(page: Page) {
  return page.evaluate(() => {
    const g = window as unknown as { __MUXT_TERM_INSTANCES__?: Array<any>; __MUXT_TERM_DEBUG_LOGS__?: Array<any> };
    const term = Array.isArray(g.__MUXT_TERM_INSTANCES__) ? g.__MUXT_TERM_INSTANCES__[g.__MUXT_TERM_INSTANCES__.length - 1] : null;
    const core = term?._core ?? null;
    const active = term?.buffer?.active ?? null;
    if (!active) {
      return {
        ok: false,
        reason: "no-active-buffer",
        hasTerm: Boolean(term),
        coreKeys: core ? Object.keys(core).slice(0, 30) : [],
        bufferServiceKeys: core?._bufferService ? Object.keys(core._bufferService).slice(0, 30) : [],
        termKeys: term ? Object.keys(term).slice(0, 30) : []
      };
    }
    const cursorY = typeof active.cursorY === "number" ? active.cursorY : -1;
    const lines: string[] = [];
    for (let i = 0; i < active.length; i += 1) {
      const line = active.getLine(i);
      lines.push(String(line?.translateToString?.(true) ?? ""));
    }
    const promptLineIndex = lines.reduce(
      (acc, line, idx) => (line.includes("/workspace/cli#") || line.includes("muxt git:(") || line.includes("➜ ") ? idx : acc),
      -1
    );
    const debugTail = (g.__MUXT_TERM_DEBUG_LOGS__ ?? []).slice(-30);
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
      cursorY,
      promptLineIndex,
      cursorPromptDelta: cursorY >= 0 && promptLineIndex >= 0 ? cursorY - promptLineIndex : null,
      visualCursorRow,
      visualCursorDelta: visualCursorRow !== null ? visualCursorRow - cursorY : null,
      cellHeight,
      canvasCursor,
      tail: lines.slice(Math.max(0, promptLineIndex - 2), promptLineIndex + 3),
      debugTail
    };
  });
}

test.describe("cursor offset repro in docker", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      (window as unknown as { __MUXT_TERM_DEBUG__?: boolean }).__MUXT_TERM_DEBUG__ = true;
    });
  });

  test("switch task and refresh should keep cursor aligned with prompt row", async ({ page, request }) => {
    const seeded = await seedProject(request);
    await page.goto(visitURL);

    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await runCommands(page, [
      "ls",
      "pwd",
      "ls"
    ]);
    await page.screenshot({ path: "../logs/cursor-repro-before-switch.png", fullPage: true });

    await selectTask(page, seeded.projectID, seeded.siblingTaskID);
    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await page.waitForTimeout(300);
    await activeTerminal(page).click();
    await page.screenshot({ path: "../logs/cursor-repro-after-switch-back.png", fullPage: true });

    const switched = await readCursorState(page);

    await page.reload();
    await selectTask(page, seeded.projectID, seeded.rootTaskID);
    await page.waitForTimeout(300);
    await activeTerminal(page).click();
    await page.screenshot({ path: "../logs/cursor-repro-after-refresh.png", fullPage: true });

    const refreshed = await readCursorState(page);

    // eslint-disable-next-line no-console
    console.log(`[cursor-repro] switch=${JSON.stringify(switched)} refresh=${JSON.stringify(refreshed)}`);
    await page.screenshot({ path: "../logs/cursor-repro-switch-refresh.png", fullPage: true });

    expect(switched.ok).toBe(true);
    expect(refreshed.ok).toBe(true);
    expect(switched.visualCursorDelta).toBe(0);
    expect(refreshed.visualCursorDelta).toBe(0);

    for (let i = 0; i < 12; i += 1) {
      await selectTask(page, seeded.projectID, seeded.siblingTaskID);
      await selectTask(page, seeded.projectID, seeded.rootTaskID);
      await page.waitForTimeout(160);
      await activeTerminal(page).click();
      const sampled = await readCursorState(page);
      expect(sampled.ok).toBe(true);
      expect(sampled.visualCursorDelta).toBe(0);
    }
  });
});
