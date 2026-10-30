#!/usr/bin/env node
// Lint/verify: gofmt, go vet, TypeScript typecheck.
import { execFileSync } from 'node:child_process';
import path from 'node:path';

const ROOT = path.resolve(import.meta.dirname, '..');
const run = (cmd, args) => execFileSync(cmd, args, { cwd: ROOT, stdio: 'inherit', env: process.env });

const unformatted = execFileSync('gofmt', ['-l', './cmd', './internal'], { cwd: ROOT })
  .toString()
  .trim();
if (unformatted) {
  console.error('✖ gofmt needed for:\n' + unformatted);
  process.exit(1);
}
console.log('✓ gofmt');

run('go', ['vet', './...']);
console.log('✓ go vet');

run('pnpm', ['--filter', 'myvideostation-web', 'typecheck']);
console.log('✓ typescript');
