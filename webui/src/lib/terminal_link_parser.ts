export type TerminalURLLink = {
  type: "url";
  text: string;
  start: number;
  end: number;
};

export type TerminalPathLink = {
  type: "path";
  text: string;
  path: string;
  line: number | null;
  col: number | null;
  start: number;
  end: number;
};

export type TerminalLinkMatch = TerminalURLLink | TerminalPathLink;

const MAX_LINE_LENGTH = 4096;
const MAX_MATCH_COUNT = 24;

const URL_RE = /https?:\/\/[\w./?%#=&:+~-]+/g;
const PATH_RE = /(?:\.\.?\/|\/)[^\s:]+(?:\:\d+)?(?:\:\d+)?|[A-Za-z0-9_.-]+\/[A-Za-z0-9_./-]+(?:\:\d+)?(?:\:\d+)?/g;

function parsePathToken(token: string) {
  const m = token.match(/^(.*?)(?::(\d+))?(?::(\d+))?$/);
  if (!m) {
    return { path: token, line: null, col: null };
  }
  return {
    path: m[1],
    line: m[2] ? Number(m[2]) : null,
    col: m[3] ? Number(m[3]) : null
  };
}

function overlaps(matches: TerminalLinkMatch[], start: number, end: number) {
  return matches.some((item) => start < item.end && end > item.start);
}

function shouldSkipPathToken(text: string) {
  return text.startsWith("http://") || text.startsWith("https://");
}

export function parseTerminalLinks(input: string): TerminalLinkMatch[] {
  const line = input.slice(0, MAX_LINE_LENGTH);
  const matches: TerminalLinkMatch[] = [];

  URL_RE.lastIndex = 0;
  let urlMatch: RegExpExecArray | null;
  while ((urlMatch = URL_RE.exec(line)) && matches.length < MAX_MATCH_COUNT) {
    const text = urlMatch[0];
    const start = urlMatch.index;
    const end = start + text.length;
    matches.push({ type: "url", text, start, end });
  }

  PATH_RE.lastIndex = 0;
  let pathMatch: RegExpExecArray | null;
  while ((pathMatch = PATH_RE.exec(line)) && matches.length < MAX_MATCH_COUNT) {
    const text = pathMatch[0];
    const start = pathMatch.index;
    const end = start + text.length;
    if (shouldSkipPathToken(text) || overlaps(matches, start, end)) {
      continue;
    }
    const parsed = parsePathToken(text);
    matches.push({
      type: "path",
      text,
      path: parsed.path,
      line: parsed.line,
      col: parsed.col,
      start,
      end
    });
  }

  matches.sort((a, b) => a.start - b.start);
  return matches;
}
