#!/usr/bin/env node
// Ensures development tools exist: air (Go live reload) and a static ffprobe.
// Safe to run repeatedly; `pnpm setup` is idempotent.
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, chmodSync, rmSync, createWriteStream } from 'node:fs';
import { arch, platform } from 'node:os';
import path from 'node:path';
import { pipeline } from 'node:stream/promises';
import { Readable } from 'node:stream';

const ROOT = path.resolve(import.meta.dirname, '..');
const TOOLS = path.join(ROOT, '.tools');
const BIN = path.join(TOOLS, 'bin');
const AIR_VERSION = 'v1.67.4';
const quiet = process.argv.includes('--quiet');

const say = (...a) => {
  if (!quiet) console.log(...a);
};
const fail = (msg) => {
  console.error(`✖ ${msg}`);
  process.exit(1);
};

mkdirSync(BIN, { recursive: true });

// ---- air -------------------------------------------------------------------
const airPath = path.join(BIN, 'air');
if (!existsSync(airPath)) {
  say('→ installing air', AIR_VERSION);
  try {
    execFileSync('go', ['install', `github.com/air-verse/air@${AIR_VERSION}`], {
      cwd: ROOT,
      stdio: quiet ? 'ignore' : 'inherit',
      env: { ...process.env, GOBIN: BIN },
    });
  } catch {
    fail('could not install air (is Go 1.27+ on PATH?)');
  }
  if (!existsSync(airPath)) fail(`air was not installed to ${airPath}`);
  say('✓ air');
} else {
  say('✓ air', path.relative(ROOT, airPath));
}

// ---- ffprobe ---------------------------------------------------------------
const ffprobePath = path.join(TOOLS, 'ffprobe');
if (existsSync(ffprobePath)) {
  say('✓ ffprobe', path.relative(ROOT, ffprobePath));
} else if (architectureUnsupported()) {
  say('! no static ffmpeg build for', `${platform()}/${arch()} — using system ffprobe if present`);
} else {
  const url = 'https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz';
  const tarball = path.join(TOOLS, 'ffmpeg.tar.xz');
  say('→ downloading static ffmpeg (dev only, ~40 MB)');
  try {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    await pipeline(Readable.fromWeb(res.body), createWriteStream(tarball));
    execFileSync('tar', ['-xJf', tarball, '-C', TOOLS, '--strip-components=1', '--wildcards', '*/ffmpeg', '*/ffprobe'], {
      stdio: quiet ? 'ignore' : 'inherit',
    });
    rmSync(tarball, { force: true });
    chmodSync(ffprobePath, 0o755);
    chmodSync(path.join(TOOLS, 'ffmpeg'), 0o755);
    say('✓ ffprobe');
  } catch (err) {
    rmSync(tarball, { force: true });
    say(`! could not download ffmpeg (${err.message}) — scanning will work without technical metadata`);
    say('  install ffmpeg yourself or use the Docker image');
  }
}

function architectureUnsupported() {
  return !(platform() === 'linux' && (arch() === 'x64' || arch() === 'amd64'));
}
