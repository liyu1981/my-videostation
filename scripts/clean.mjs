#!/usr/bin/env node
// Removes build artifacts. Never deletes ./data (your library DB) unless --data is passed.
import { rmSync } from 'node:fs';
import path from 'node:path';

const ROOT = path.resolve(import.meta.dirname, '..');
const targets = ['bin', '.air', 'internal/web/dist', 'web/dist'];
if (process.argv.includes('--data')) targets.push('data');

for (const t of targets) {
  rmSync(path.join(ROOT, t), { recursive: true, force: true });
  console.log(`✓ removed ${t}`);
}
if (!process.argv.includes('--data')) console.log('  (data/ kept — pass --data to delete it too)');
