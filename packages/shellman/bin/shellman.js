#!/usr/bin/env node

import { spawn } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { homedir, platform, arch } from "node:os";
import path from "node:path";
import process from "node:process";

function resolveTargetTriple() {
  const os = platform();
  const cpu = arch();

  if ((os === "darwin" || os === "linux") && (cpu === "arm64" || cpu === "x64")) {
    return `${os}-${cpu === "x64" ? "amd64" : "arm64"}`;
  }

  return "";
}

function getPackageMeta() {
  const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  const version = String(pkg.version ?? "").trim();
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

function resolveInstalledBinary(channel, pkgVersion) {
  const triple = resolveTargetTriple();
  if (!triple) {
    console.error(`shellman: unsupported platform ${platform()}/${arch()} (only darwin/linux + arm64/x64 supported)`);
    process.exit(1);
  }

  const channelRoot = path.join(homedir(), ".shellman", "channels", channel, triple);
  const channelBin = path.join(channelRoot, "bin", "shellman");
  if (existsSync(channelBin)) {
    return {
      binPath: channelBin,
      versionFile: path.join(channelRoot, "VERSION")
    };
  }

  const legacyRoot = path.join(homedir(), ".shellman", "versions", `v${pkgVersion}`, triple);
  const legacyBin = path.join(legacyRoot, "bin", "shellman");
  if (existsSync(legacyBin)) {
    return {
      binPath: legacyBin,
      versionFile: path.join(legacyRoot, "VERSION")
    };
  }

  console.error("shellman: prebuilt binary is not installed. Reinstall package:");
  console.error(`  npm install -g ${channel === "dev" ? "shellman@dev" : "shellman"}`);
  process.exit(1);
}

function readInstalledVersion(fallbackVersion, versionFile) {
  if (!existsSync(versionFile)) {
    return fallbackVersion;
  }
  const raw = String(readFileSync(versionFile, "utf8")).trim();
  if (!raw) {
    return fallbackVersion;
  }
  return raw.replace(/^v/, "");
}

function shouldSkipUpdateCheck() {
  return String(process.env.SHELLMAN_NO_UPDATE_CHECK ?? "").trim() === "1";
}

function getUpdateCheckIntervalSec() {
  const raw = String(process.env.SHELLMAN_UPDATE_CHECK_INTERVAL_SEC ?? "").trim();
  const parsed = Number(raw);
  if (Number.isFinite(parsed) && parsed > 0) {
    return Math.floor(parsed);
  }
  return 43200;
}

function loadUpdateCache(cachePath) {
  if (!existsSync(cachePath)) {
    return { checkedAt: 0, lastNotifiedVersion: "" };
  }
  try {
    const parsed = JSON.parse(readFileSync(cachePath, "utf8"));
    return {
      checkedAt: Number(parsed.checkedAt ?? 0),
      lastNotifiedVersion: String(parsed.lastNotifiedVersion ?? "")
    };
  } catch {
    return { checkedAt: 0, lastNotifiedVersion: "" };
  }
}

function saveUpdateCache(cachePath, payload) {
  mkdirSync(path.dirname(cachePath), { recursive: true });
  writeFileSync(cachePath, JSON.stringify(payload), "utf8");
}

async function checkForUpdateNotice({ channel, installedVersion }) {
  if (shouldSkipUpdateCheck()) {
    return;
  }

  const cachePath = path.join(homedir(), ".shellman", "cache", `update-${channel}.json`);
  const nowSec = Math.floor(Date.now() / 1000);
  const intervalSec = getUpdateCheckIntervalSec();
  const cache = loadUpdateCache(cachePath);
  if (nowSec - cache.checkedAt < intervalSec) {
    return;
  }

  try {
    const manifestURL = getManifestURL(channel);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 2500);
    const res = await fetch(manifestURL, {
      headers: { Accept: "application/json" },
      signal: controller.signal
    });
    clearTimeout(timer);

    if (!res.ok) {
      saveUpdateCache(cachePath, { checkedAt: nowSec, lastNotifiedVersion: cache.lastNotifiedVersion });
      return;
    }

    const manifest = await res.json();
    const remoteVersion = String(manifest?.version ?? "").trim().replace(/^v/, "");
    if (!remoteVersion) {
      saveUpdateCache(cachePath, { checkedAt: nowSec, lastNotifiedVersion: cache.lastNotifiedVersion });
      return;
    }

    if (remoteVersion !== installedVersion && cache.lastNotifiedVersion !== remoteVersion) {
      const upgradeCmd = channel === "dev" ? "npm install -g shellman@dev" : "npm install -g shellman";
      console.error(`shellman: update available (${channel}) current=${installedVersion} latest=${remoteVersion}`);
      console.error(`shellman: run \`${upgradeCmd}\``);
      saveUpdateCache(cachePath, { checkedAt: nowSec, lastNotifiedVersion: remoteVersion });
      return;
    }

    saveUpdateCache(cachePath, { checkedAt: nowSec, lastNotifiedVersion: cache.lastNotifiedVersion });
  } catch {
    saveUpdateCache(cachePath, { checkedAt: nowSec, lastNotifiedVersion: cache.lastNotifiedVersion });
  }
}

const { version: pkgVersion } = getPackageMeta();
const channel = getChannel(pkgVersion);
const resolved = resolveInstalledBinary(channel, pkgVersion);
const installedVersion = readInstalledVersion(pkgVersion, resolved.versionFile);

void checkForUpdateNotice({ channel, installedVersion });

const child = spawn(resolved.binPath, process.argv.slice(2), {
  stdio: "inherit",
  env: process.env,
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});

child.on("error", (error) => {
  console.error(`shellman: failed to start binary: ${error.message}`);
  process.exit(1);
});
