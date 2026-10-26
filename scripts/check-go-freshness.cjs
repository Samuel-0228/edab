#!/usr/bin/env node

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const ROOT_DIR = path.join(__dirname, '..');
const POLICY_FILE = process.env.POLICY_FILE || path.join(ROOT_DIR, '.dependency-policy.json');

let policy = {};
if (fs.existsSync(POLICY_FILE)) {
  try {
    policy = JSON.parse(fs.readFileSync(POLICY_FILE, 'utf8'));
  } catch (e) {
    console.error('Warning: could not parse .dependency-policy.json:', e.message);
  }
}

const goPolicy = policy.go || {
  allowUpdates: []
};

try {
  const output = execSync('go list -u -m -json all', {
    cwd: ROOT_DIR,
    encoding: 'utf8'
  });

  const jsonBlocks = output
    .split(/\n(?=\{)/g)
    .map(s => s.trim())
    .filter(Boolean);

  const directDeps = [];
  const outdatedDirect = [];

  for (const block of jsonBlocks) {
    let mod;
    try {
      mod = JSON.parse(block);
    } catch {
      continue;
    }

    if (mod.Main) continue;
    if (mod.Indirect) continue; // Only enforce policy on direct dependencies

    directDeps.push(mod);

    if (mod.Update) {
      const allowed = (goPolicy.allowUpdates || []).some(entry => {
        if (typeof entry === 'string') return entry === mod.Path;
        return entry.path === mod.Path && entry.pinnedVersion === mod.Version;
      });

      if (!allowed) {
        outdatedDirect.push({
          path: mod.Path,
          current: mod.Version,
          latest: mod.Update.Version,
          reason: 'Direct dependency has an unapproved newer version available upstream'
        });
      }
    }
  }

  console.log(`Audited ${directDeps.length} direct Go dependencies:`);
  for (const dep of directDeps) {
    console.log(`  ✓ ${dep.Path}@${dep.Version}`);
  }

  if (outdatedDirect.length > 0) {
    console.error('\n❌ Go Dependency Freshness Policy Violation:');
    console.error('The following direct dependencies require attention:');
    for (const item of outdatedDirect) {
      console.error(`  - ${item.path}: current ${item.current} -> available ${item.latest}`);
      console.error(`    ${item.reason}`);
    }
    console.error('\nAction required: Run "go get <module>@<version>" or document justification in .dependency-policy.json.');
    process.exit(1);
  }

  console.log('✅ All direct Go dependencies satisfy freshness policy.');
} catch (err) {
  console.error('Failed to check Go dependency freshness:', err.message);
  process.exit(1);
}
