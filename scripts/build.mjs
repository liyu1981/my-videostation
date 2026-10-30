#!/usr/bin/env node
// Production build: web bundle -> embedded dist -> single static binary.
import { execFileSync } from 'node:child_process';
import { cpSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
import path from 'node:path';

const ROOT = path.resolve(import.meta.dirname, '..');
const run = (cmd, args, env = {}) =>
  execFileSync(cmd, args, { cwd: ROOT, stdio: 'inherit', env: { ...process.env, ...env } });

const git = (args, fallback) => {
  try {
    return execFileSync('git', args, { cwd: ROOT, stdio: ['ignore', 'pipe', 'ignore'] })
      .toString()
      .trim();
  } catch {
    return fallback;
  }
};

const pkg = JSON.parse(readFileSync(path.join(ROOT, 'web', 'package.json'), 'utf8'));
const version = process.env.VERSION || git(['describe', '--tags', '--always', '--dirty'], pkg.version || 'dev');
const commit = process.env.COMMIT || git(['rev-parse', '--short', 'HEAD'], 'none');
const date = process.env.BUILD_DATE || new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');

console.log(`→ building web UI`);
run('pnpm', ['--filter', 'myvideostation-web', 'build']);

const dist = path.join(ROOT, 'internal', 'web', 'dist');
rmSync(dist, { recursive: true, force: true });
mkdirSync(dist, { recursive: true });
cpSync(path.join(ROOT, 'web', 'dist'), dist, { recursive: true });

console.log(`→ compiling binary (version ${version})`);
const ldflags = [
  '-s',
  '-w',
  `-X github.com/liyu1981/myvideostation/internal/buildinfo.Version=${version}`,
  `-X github.com/liyu1981/myvideostation/internal/buildinfo.Commit=${commit}`,
  `-X github.com/liyu1981/myvideostation/internal/buildinfo.Date=${date}`,
].join(' ');
run('go', ['build', '-tags', 'embedui', '-trimpath', '-ldflags', ldflags, '-o', 'bin/mvs', './cmd/mvs'], {
  CGO_ENABLED: '0',
});
console.log(`✓ bin/mvs (${version}) — UI embedded`);
