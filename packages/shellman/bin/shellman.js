#!/usr/bin/env node

import { spawn } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
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

function resolveInstalledBinary() {
  const triple = resolveTargetTriple();
  if (!triple) {
    console.error(`shellman: unsupported platform ${platform()}/${arch()} (only darwin/linux + arm64/x64 supported)`);
    process.exit(1);
  }

  const pkgVersion = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8")).version;
  const installRoot = path.join(homedir(), ".shellman", "versions", `v${pkgVersion}`, triple, "bin");
  const binPath = path.join(installRoot, "shellman");

  if (!existsSync(binPath)) {
    console.error("shellman: prebuilt binary is not installed. Reinstall package:");
    console.error("  npm install -g shellman");
    process.exit(1);
  }

  return binPath;
}

const bin = resolveInstalledBinary();
const child = spawn(bin, process.argv.slice(2), {
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
