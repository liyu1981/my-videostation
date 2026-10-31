#!/usr/bin/env node
// Dev build: compile the SPA, embed it, then build the single Go binary that air runs.
// Triggered by air on any Go/SQL/frontend change.
import { execFileSync } from 'node:child_process';
import { cpSync, mkdirSync, rmSync } from 'node:fs';
import path from 'node:path';

const ROOT = path.resolve(import.meta.dirname, '..');
const run = (cmd, args, env = {}) =>
  execFileSync(cmd, args, { cwd: ROOT, stdio: 'inherit', env: { ...process.env, ...env } });

const started = Date.now();

// 1) frontend bundle
run('pnpm', ['--filter', 'myvideostation-web', 'build']);

// 2) copy into the embedded directory (gitignored, produced by this script)
const dist = path.join(ROOT, 'internal', 'web', 'dist');
rmSync(dist, { recursive: true, force: true });
mkdirSync(dist, { recursive: true });
cpSync(path.join(ROOT, 'web', 'dist'), dist, { recursive: true });

// 3) single binary with the UI embedded (dev: no -s -w / ldflags for speed)
run('go', ['build', '-tags', 'embedui', '-o', path.join('.air', 'mvs'), './cmd/mvs'], {
  CGO_ENABLED: '0',
});

console.log(`✓ dev binary rebuilt in ${Date.now() - started}ms (UI embedded)`);
