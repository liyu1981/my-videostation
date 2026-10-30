#!/usr/bin/env node
// Runs air with the repo config, from .tools/bin (installed by `pnpm setup`).
import { existsSync } from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';

const ROOT = path.resolve(import.meta.dirname, '..');
const air = path.join(ROOT, '.tools', 'bin', 'air');

if (!existsSync(air)) {
  console.error('✖ air is not installed. Run `pnpm setup` first.');
  process.exit(1);
}

const child = spawn(air, ['-c', '.air.toml'], { cwd: ROOT, stdio: 'inherit' });
for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => child.kill(sig));
}
child.on('exit', (code) => process.exit(code ?? 1));
