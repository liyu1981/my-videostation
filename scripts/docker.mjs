#!/usr/bin/env node
// Builds the Docker image with version metadata injected.
import { execFileSync } from 'node:child_process';
import path from 'node:path';

const ROOT = path.resolve(import.meta.dirname, '..');
const git = (args, fallback) => {
  try {
    return execFileSync('git', args, { cwd: ROOT, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
  } catch {
    return fallback;
  }
};

const version = process.env.VERSION || git(['describe', '--tags', '--always', '--dirty'], 'dev');
const commit = process.env.COMMIT || git(['rev-parse', '--short', 'HEAD'], 'none');
const date = process.env.BUILD_DATE || new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
const tag = process.env.IMAGE || `myvideostation:${version}`;

console.log(`→ docker build ${tag}`);
execFileSync(
  'docker',
  [
    'build',
    '-f', 'deployments/Dockerfile',
    '--build-arg', `VERSION=${version}`,
    '--build-arg', `COMMIT=${commit}`,
    '--build-arg', `BUILD_DATE=${date}`,
    '-t', tag,
    '.',
  ],
  { cwd: ROOT, stdio: 'inherit' },
);
console.log(`✓ ${tag}`);
