import { execSync } from 'child_process';
import path from 'path';
import fs from 'fs';

const root = path.resolve(__dirname, '..');
const specPath = path.join(root, 'openapi', 'secretpad.openapi.json');
const outPath = path.join(root, 'packages', 'api-client', 'src', 'generated', 'secretpad.d.ts');

if (!fs.existsSync(specPath)) {
  console.error(`[codegen] OpenAPI spec not found: ${specPath}`);
  console.error('[codegen] Please start the backend and run:');
  console.error(`  curl http://127.0.0.1:8080/v3/api-docs > ${specPath}`);
  process.exit(1);
}

const cmd = [
  'pnpm openapi-typescript',
  `"${specPath}"`,
  '-o',
  `"${outPath}"`,
  '--alphabetize',
].join(' ');

console.log(`[codegen] ${cmd}`);
execSync(cmd, { stdio: 'inherit', cwd: root });
console.log(`[codegen] Generated: ${outPath}`);
