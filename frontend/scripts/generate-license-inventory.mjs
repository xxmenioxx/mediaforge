import { readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const lock = JSON.parse(await readFile(path.join(frontendRoot, 'package-lock.json'), 'utf8'));
const outputRoot = path.join(frontendRoot, 'licenses', 'npm');
const args = new Set(process.argv.slice(2));
const checkOnly = args.has('--check');
const requireComplete = args.has('--require-complete');
const packageEntries = Object.entries(lock.packages ?? {})
  .filter(([packagePath, entry]) => packagePath.startsWith('node_modules/') && entry.dev !== true)
  .sort(([left], [right]) => left.localeCompare(right));
const unresolved = [];
const inventory = [];
const notices = [
  'MVForge web runtime third-party notices',
  `Source: frontend/package-lock.json (lockfileVersion ${lock.lockfileVersion})`,
  'Scope: resolved npm packages not marked as development-only in the lockfile.',
  'This inventory preserves declared metadata and bundled notice text; it is not a legal conclusion.',
  '',
];

function isNoticeFile(name) {
  return /^(un)?licen[cs]e(?:[._ -].*)?$|^copying(?:[._ -].*)?$|^notice(?:[._ -].*)?$|^third[-_. ]party[-_. ]notices?(?:[._ -].*)?$/i.test(name);
}

async function noticeFiles(directory, relative = '') {
  const files = [];
  let entries;
  try {
    entries = await readdir(path.join(directory, relative), { withFileTypes: true });
  } catch {
    return files;
  }
  for (const entry of entries.sort((left, right) => left.name.localeCompare(right.name))) {
    if (entry.name === 'node_modules') continue;
    const child = path.join(relative, entry.name);
    if (entry.isDirectory()) {
      files.push(...await noticeFiles(directory, child));
    } else if (entry.isFile() && isNoticeFile(entry.name)) {
      files.push(child.split(path.sep).join('/'));
    }
  }
  return files;
}

for (const [packagePath, entry] of packageEntries) {
  const packageDirectory = path.join(frontendRoot, packagePath);
  const files = await noticeFiles(packageDirectory);
  const packageRecord = {
    path: packagePath,
    name: entry.name ?? packagePath.slice('node_modules/'.length),
    version: entry.version ?? 'unknown',
    license: entry.license ?? null,
    licenseFiles: files,
    licenseTextIncluded: files.length > 0,
  };
  inventory.push(packageRecord);

  if (!packageRecord.license || files.length === 0) {
    unresolved.push({ path: packagePath, name: packageRecord.name, version: packageRecord.version, reason: !packageRecord.license ? 'missing lockfile license metadata' : 'no bundled license or notice text found' });
  }

  for (const noticePath of files) {
    const content = await readFile(path.join(packageDirectory, noticePath), 'utf8');
    notices.push(`----- ${packageRecord.name}@${packageRecord.version} (${packageRecord.license ?? 'license unresolved'}) -----`);
    notices.push(`Source: ${packagePath}/${noticePath}`);
    notices.push(content.replace(/\s+$/, ''));
    notices.push('');
  }
}

const outputs = new Map([
  ['inventory.json', `${JSON.stringify({ lockfileVersion: lock.lockfileVersion, scope: 'production-and-shared-lockfile-packages', packageCount: inventory.length, unresolved, packages: inventory }, null, 2)}\n`],
  ['THIRD_PARTY_NOTICES.txt', `${notices.join('\n')}\n`],
]);
let stale = false;
for (const [name, content] of outputs) {
  const outputPath = path.join(outputRoot, name);
  if (checkOnly) {
    try {
      const existing = await readFile(outputPath, 'utf8');
      if (existing !== content) {
        console.error(`stale or missing npm license evidence: ${path.relative(frontendRoot, outputPath)}`);
        stale = true;
      }
    } catch {
      console.error(`missing npm license evidence: ${path.relative(frontendRoot, outputPath)}`);
      stale = true;
    }
  } else {
    await (await import('node:fs/promises')).mkdir(outputRoot, { recursive: true });
    await writeFile(outputPath, content);
  }
}

console.log(`npm runtime license inventory: ${inventory.length} packages, ${unresolved.length} unresolved`);
if (requireComplete && unresolved.length > 0) process.exitCode = 1;
if (checkOnly && stale) process.exitCode = 1;
