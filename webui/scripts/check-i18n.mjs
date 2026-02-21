#!/usr/bin/env bun

import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { detectHardStrings } from "@cybersailor/i18n-detect-vue/dist/index.js";
import { checkLocalizedKeys } from "@cybersailor/i18n-detect-vue/dist/check/index.js";

const argv = process.argv.slice(2);
const mode = getArgValue("--mode");
const root = getArgValue("--root") ?? "src";
const lang = getArgValue("--lang");

if (!mode || (mode !== "keys" && mode !== "hardcoded")) {
  console.error("Usage: bun webui/scripts/check-i18n.mjs --mode <keys|hardcoded> [--root <dir>] [--lang <json>]");
  process.exit(1);
}

if (mode === "keys" && !lang) {
  console.error("Missing required argument: --lang <json>");
  process.exit(1);
}

const files = walkFiles(root).filter((file) => isSourceFile(file) && !isIgnoredFile(file));
const hardcodedFiles = files.filter((file) => file.endsWith(".vue"));
let hasIssue = false;

if (mode === "keys") {
  for (const file of files) {
    const missing = checkLocalizedKeys(file, { langJsonPath: lang });
    for (const item of missing) {
      hasIssue = true;
      console.log(`${item.file}:${item.line}\t${item.key}`);
    }
  }
}

if (mode === "hardcoded") {
  for (const file of hardcodedFiles) {
    const detections = detectHardStrings(file);
    for (const detection of detections) {
      const normalizedText = String(detection.text ?? "").replace(/\s+/g, " ").trim();
      if (!normalizedText) {
        continue;
      }
      if (shouldIgnoreDetection(normalizedText)) {
        continue;
      }
      hasIssue = true;
      console.log(`${file}:${lineNumber(file, detection.start)}\t${normalizedText}\t${detection.source}\tlow`);
    }
  }
}

if (hasIssue) {
  process.exit(1);
}

function getArgValue(flag) {
  const idx = argv.indexOf(flag);
  if (idx === -1 || idx === argv.length - 1) {
    return null;
  }
  return argv[idx + 1];
}

function walkFiles(dir) {
  const out = [];
  const entries = readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (isIgnoredDir(fullPath)) {
        continue;
      }
      out.push(...walkFiles(fullPath));
      continue;
    }
    if (entry.isFile()) {
      out.push(fullPath);
    }
  }
  return out;
}

function lineNumber(filePath, start) {
  if (!Number.isFinite(start) || start < 0) {
    return 1;
  }
  const content = readFileSync(filePath, "utf-8");
  return content.slice(0, start).split("\n").length;
}

function shouldIgnoreDetection(text) {
  if (/^(Unknown error|nil|null|true|false|undefined|console|debug|error|warn|info|log)$/.test(text)) {
    return true;
  }
  if (/^[\s]*$/.test(text)) {
    return true;
  }
  if (/^[\p{P}]{1,3}$/u.test(text)) {
    return true;
  }
  if (text.includes("props.") || text.includes("{{") || text.includes("||") || text.includes("(min-width:") || text.includes("t('") || text.includes('t("')) {
    return true;
  }
  if (text.includes("${")) {
    return true;
  }
  if (text.includes(" ") && !/[A-Z]/.test(text) && /[-:/[\]()%]/.test(text)) {
    return true;
  }
  if (/^(\$\{.*\}|diff --git|\|)$/.test(text)) {
    return true;
  }
  if (/^(Esc|Tab|Enter|Backspace|Up|Down|Left|Right|Ctrl|Alt)$/.test(text)) {
    return true;
  }
  return false;
}

function isSourceFile(filePath) {
  return filePath.endsWith(".vue") || filePath.endsWith(".ts") || filePath.endsWith(".js");
}

function isIgnoredFile(filePath) {
  return filePath.endsWith(".spec.ts") || filePath.endsWith(".test.ts");
}

function isIgnoredDir(dirPath) {
  const normalized = dirPath.split(path.sep).join("/");
  return (
    normalized.includes("/generated/") ||
    normalized.includes("/components/ui/") ||
    normalized.includes("/components/ai-elements/")
  );
}
