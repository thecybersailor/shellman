#!/usr/bin/env node

import { createHash } from "node:crypto";
import { chmodSync, createReadStream, createWriteStream, existsSync, mkdirSync, readFileSync, rmSync } from "node:fs";
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

function getPackageMeta() {
  const packageJSON = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  const version = String(packageJSON.version ?? "").trim();
  return { version };
}

function getChannel(pkgVersion) {
  const forced = String(process.env.SHELLMAN_RELEASE_CHANNEL ?? "").trim();
  if (forced === "stable" || forced === "dev") {
    return forced;
  }
  return pkgVersion.includes("-dev") ? "dev" : "stable";
}

function getRepo() {
  const raw = (process.env.SHELLMAN_GITHUB_REPO || "thecybersailor/shellman").trim();
  return raw || "thecybersailor/shellman";
}

function getManifestURL(channel) {
  const custom = String(process.env.SHELLMAN_RELEASE_MANIFEST_URL ?? "").trim();
  if (custom) {
    return custom;
  }
  const repo = getRepo();
  if (channel === "dev") {
    return `https://github.com/${repo}/releases/download/dev-main/release.json`;
  }
  return `https://github.com/${repo}/releases/latest/download/release.json`;
}

function getInstallRoot(channel, triple) {
  return path.join(homedir(), ".shellman", "channels", channel, triple);
}

async function download(url, outFile) {
  const res = await fetch(url);
  if (!res.ok || !res.body) {
    throw new Error(`download failed ${res.status} ${res.statusText}: ${url}`);
  }
  await pipeline(Readable.fromWeb(res.body), createWriteStream(outFile));
}

async function fetchJSON(url) {
  const res = await fetch(url, { headers: { Accept: "application/json" } });
  if (!res.ok) {
    throw new Error(`manifest request failed ${res.status} ${res.statusText}: ${url}`);
  }
  return res.json();
}

function sha256File(filePath) {
  const hash = createHash("sha256");
  hash.update(readFileSync(filePath));
  return hash.digest("hex");
}

function resolveReleaseAsset(manifest, triple) {
  const version = String(manifest?.version ?? "").trim();
  const baseURL = String(manifest?.base_url ?? "").replace(/\/+$/, "");
  const artifacts = Array.isArray(manifest?.artifacts) ? manifest.artifacts : [];
  const hit = artifacts.find((item) => String(item?.target ?? "") === triple);
  if (!version) {
    throw new Error("invalid release manifest: missing version");
  }
  if (!baseURL) {
    throw new Error("invalid release manifest: missing base_url");
  }
  if (!hit) {
    throw new Error(`release manifest missing target: ${triple}`);
  }
  const file = String(hit.file ?? "").trim();
  const sha256 = String(hit.sha256 ?? "").trim().toLowerCase();
  if (!file || !sha256) {
    throw new Error(`release manifest target incomplete: ${triple}`);
  }
  return {
    version,
    file,
    sha256,
    url: `${baseURL}/${file}`
  };
}

async function main() {
  if ((process.env.SHELLMAN_SKIP_POSTINSTALL || "").trim() === "1") {
    console.log("shellman: skip postinstall by SHELLMAN_SKIP_POSTINSTALL=1");
    return;
  }
  const forcePostinstall = String(process.env.SHELLMAN_FORCE_POSTINSTALL ?? "").trim() === "1";
  const isGlobalInstall = String(process.env.npm_config_global ?? "").trim() === "true";
  if (!forcePostinstall && !isGlobalInstall) {
    console.log("shellman: skip postinstall for non-global install");
    return;
  }

  const triple = resolveTargetTriple();
  if (!triple) {
    console.error(`shellman: unsupported platform ${platform()}/${arch()} (only darwin/linux + arm64/x64 supported)`);
    process.exit(1);
  }

  const { version: pkgVersion } = getPackageMeta();
  const channel = getChannel(pkgVersion);
  const installRoot = getInstallRoot(channel, triple);
  const binPath = path.join(installRoot, "bin", "shellman");

  if (existsSync(binPath)) {
    console.log(`shellman: binary already installed at ${binPath}`);
    return;
  }

  const manifestURL = getManifestURL(channel);
  console.log(`shellman: reading release manifest (${channel}) ${manifestURL}`);
  const manifest = await fetchJSON(manifestURL);
  const releaseAsset = resolveReleaseAsset(manifest, triple);

  const tmpRoot = path.join(homedir(), ".shellman", "tmp", `${channel}-${triple}`);
  rmSync(tmpRoot, { recursive: true, force: true });
  mkdirSync(tmpRoot, { recursive: true });

  const zipPath = path.join(tmpRoot, releaseAsset.file);
  console.log(`shellman: downloading ${releaseAsset.url}`);
  await download(releaseAsset.url, zipPath);

  const actual = sha256File(zipPath);
  if (actual !== releaseAsset.sha256) {
    throw new Error(`checksum mismatch for ${releaseAsset.file}`);
  }

  rmSync(installRoot, { recursive: true, force: true });
  mkdirSync(installRoot, { recursive: true });
  await pipeline(createReadStream(zipPath), unzipper.Extract({ path: installRoot }));

  if (!existsSync(binPath)) {
    throw new Error(`installed package missing binary: ${binPath}`);
  }
  chmodSync(binPath, 0o755);

  console.log(`shellman: installed ${releaseAsset.version} (${channel}) for ${triple}`);
}

main().catch((error) => {
  console.error(`shellman: postinstall failed: ${error.message}`);
  process.exit(1);
});
