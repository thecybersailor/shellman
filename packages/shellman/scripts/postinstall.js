#!/usr/bin/env node

import { createHash } from "node:crypto";
import { createReadStream, createWriteStream, existsSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { Readable } from "node:stream";
import { pipeline } from "node:stream/promises";
import { homedir, platform, arch } from "node:os";
import path from "node:path";
import process from "node:process";
import * as unzipper from "unzipper";

function resolveTargetTriple() {
  const os = platform();
  const cpu = arch();
  if ((os === "darwin" || os === "linux") && (cpu === "arm64" || cpu === "x64")) {
    return `${os}-${cpu === "x64" ? "amd64" : "arm64"}`;
  }
  return "";
}

function getPackageVersion() {
  const packageJSON = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  return packageJSON.version;
}

function getRepo() {
  const raw = (process.env.SHELLMAN_GITHUB_REPO || "cybersailor/shellman-project").trim();
  return raw || "cybersailor/shellman-project";
}

function getDownloadBase(version) {
  const customBase = (process.env.SHELLMAN_DOWNLOAD_BASE_URL || "").trim();
  if (customBase) {
    return customBase.replace(/\/+$/, "");
  }
  return `https://github.com/${getRepo()}/releases/download/v${version}`;
}

async function download(url, outFile) {
  const res = await fetch(url);
  if (!res.ok || !res.body) {
    throw new Error(`download failed ${res.status} ${res.statusText}: ${url}`);
  }
  await pipeline(Readable.fromWeb(res.body), createWriteStream(outFile));
}

function sha256File(filePath) {
  const hash = createHash("sha256");
  hash.update(readFileSync(filePath));
  return hash.digest("hex");
}

function parseSHA256Sums(filePath) {
  const lines = readFileSync(filePath, "utf8").split(/\r?\n/);
  const out = new Map();
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const m = trimmed.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (!m) continue;
    out.set(m[2].trim(), m[1].toLowerCase());
  }
  return out;
}

async function main() {
  if ((process.env.SHELLMAN_SKIP_POSTINSTALL || "").trim() === "1") {
    console.log("shellman: skip postinstall by SHELLMAN_SKIP_POSTINSTALL=1");
    return;
  }

  const triple = resolveTargetTriple();
  if (!triple) {
    console.error(`shellman: unsupported platform ${platform()}/${arch()} (only darwin/linux + arm64/x64 supported)`);
    process.exit(1);
  }

  const version = getPackageVersion();
  const assetName = `shellman-v${version}-${triple}.zip`;
  const checksumsName = "SHA256SUMS";
  const base = getDownloadBase(version);

  const installRoot = path.join(homedir(), ".shellman", "versions", `v${version}`, triple);
  const binPath = path.join(installRoot, "bin", "shellman");
  if (existsSync(binPath)) {
    console.log(`shellman: binary already installed at ${binPath}`);
    return;
  }

  const tmpRoot = path.join(homedir(), ".shellman", "tmp", `v${version}-${triple}`);
  rmSync(tmpRoot, { recursive: true, force: true });
  mkdirSync(tmpRoot, { recursive: true });

  const zipPath = path.join(tmpRoot, assetName);
  const sumsPath = path.join(tmpRoot, checksumsName);
  const zipURL = `${base}/${assetName}`;
  const sumsURL = `${base}/${checksumsName}`;

  console.log(`shellman: downloading ${zipURL}`);
  await download(zipURL, zipPath);
  await download(sumsURL, sumsPath);

  const parsed = parseSHA256Sums(sumsPath);
  const expected = parsed.get(assetName);
  if (!expected) {
    throw new Error(`missing checksum entry for ${assetName}`);
  }
  const actual = sha256File(zipPath);
  if (actual !== expected) {
    throw new Error(`checksum mismatch for ${assetName}`);
  }

  rmSync(installRoot, { recursive: true, force: true });
  mkdirSync(installRoot, { recursive: true });
  await pipeline(
    createReadStream(zipPath),
    unzipper.Extract({ path: installRoot }),
  );

  if (!existsSync(binPath)) {
    throw new Error(`installed package missing binary: ${binPath}`);
  }

  console.log(`shellman: installed v${version} for ${triple}`);
}

main().catch((error) => {
  console.error(`shellman: postinstall failed: ${error.message}`);
  process.exit(1);
});
