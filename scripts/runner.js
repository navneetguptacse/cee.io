#!/usr/bin/env node

const { spawn } = require("child_process");
const path = require("path");
const fs = require("fs");

const ext = process.platform === "win32" ? ".exe" : "";
const binName = `cee${ext}`;

// Candidate locations for the native binary
const candidates = [
  path.join(__dirname, "..", "bin", binName),
  path.join(__dirname, binName),
  path.join(__dirname, "..", binName),
];

let binPath = candidates.find((p) => fs.existsSync(p));

if (!binPath) {
  console.error(`Error: cee native binary not found.`);
  console.error('Please run "npm rebuild" or reinstall the package:');
  console.error("  npm install -g cee-cli");
  process.exit(1);
}

const child = spawn(binPath, process.argv.slice(2), {
  stdio: "inherit",
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
  } else {
    process.exit(code ?? 0);
  }
});
