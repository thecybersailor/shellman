export type ResolvedPathLink = {
  safePath: string;
  line: number | null;
  col: number | null;
};

type ParsedToken = {
  path: string;
  line: number | null;
  col: number | null;
};

function parsePathToken(raw: string): ParsedToken | null {
  const text = String(raw ?? "").trim();
  if (!text) {
    return null;
  }
  const m = text.match(/^(.*?)(?::(\d+))?(?::(\d+))?$/);
  if (!m) {
    return { path: text, line: null, col: null };
  }
  return {
    path: String(m[1] ?? "").trim(),
    line: m[2] ? Number(m[2]) : null,
    col: m[3] ? Number(m[3]) : null
  };
}

function normalizeAbsolutePath(input: string): string {
  const pieces = input.split("/");
  const out: string[] = [];
  for (const piece of pieces) {
    if (!piece || piece === ".") {
      continue;
    }
    if (piece === "..") {
      out.pop();
      continue;
    }
    out.push(piece);
  }
  return `/${out.join("/")}`;
}

function resolveAgainstRoot(pathText: string, root: string): string | null {
  if (!root.startsWith("/")) {
    return null;
  }
  if (pathText.startsWith("/")) {
    return normalizeAbsolutePath(pathText);
  }
  return normalizeAbsolutePath(`${root.replace(/\/$/, "")}/${pathText}`);
}

function isInsideRoot(absolutePath: string, root: string): boolean {
  const normalizedRoot = normalizeAbsolutePath(root);
  return absolutePath === normalizedRoot || absolutePath.startsWith(`${normalizedRoot}/`);
}

export function resolvePathLinkInProject(raw: string, projectRoot: string): ResolvedPathLink | null {
  const parsed = parsePathToken(raw);
  if (!parsed || !parsed.path) {
    return null;
  }
  const normalizedRoot = normalizeAbsolutePath(String(projectRoot ?? "").trim());
  if (!normalizedRoot || normalizedRoot === "/") {
    return null;
  }

  const absolutePath = resolveAgainstRoot(parsed.path, normalizedRoot);
  if (!absolutePath || !isInsideRoot(absolutePath, normalizedRoot)) {
    return null;
  }

  const safePath = absolutePath.slice(normalizedRoot.length).replace(/^\//, "");
  if (!safePath) {
    return null;
  }

  return {
    safePath,
    line: parsed.line,
    col: parsed.col
  };
}
