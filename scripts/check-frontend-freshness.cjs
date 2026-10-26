#!/usr/bin/env node

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const ROOT_DIR = path.join(__dirname, '..');
const FRONTEND_DIR = path.join(ROOT_DIR, 'frontend');
const POLICY_FILE = process.env.POLICY_FILE || path.join(ROOT_DIR, '.dependency-policy.json');

let policy = {};
if (fs.existsSync(POLICY_FILE)) {
  try {
    policy = JSON.parse(fs.readFileSync(POLICY_FILE, 'utf8'));
  } catch (e) {
    console.error('Warning: could not parse .dependency-policy.json:', e.message);
  }
}

const frontendPolicy = policy.frontend || {
  pinnedMajorTracks: {},
  allowOutdated: []
};

// 1. Verify package.json dependencies are accounted for
const pkgJson = JSON.parse(fs.readFileSync(path.join(FRONTEND_DIR, 'package.json'), 'utf8'));
const allDeps = {
  ...pkgJson.dependencies,
  ...pkgJson.devDependencies
};

console.log(`Audited ${Object.keys(allDeps).length} frontend dependencies from package.json:`);

// 2. Run npm outdated --json
let outdated = {};
try {
  const stdout = execSync('npm outdated --json', {
    cwd: FRONTEND_DIR,
    encoding: 'utf8',
    stdio: ['pipe', 'pipe', 'ignore']
  });
  if (stdout.trim()) {
    outdated = JSON.parse(stdout);
  }
} catch (e) {
  // npm outdated exits with 1 when outdated packages exist
  if (e.stdout) {
    try {
      outdated = JSON.parse(e.stdout.toString('utf8'));
    } catch {
      outdated = {};
    }
  }
}

const violations = [];
const approvedPins = frontendPolicy.pinnedMajorTracks || {};

for (const [pkg, info] of Object.entries(outdated)) {
  // Rule 1: Strict semver drift: current must equal wanted
  if (info.current !== info.wanted) {
    violations.push({
      pkg,
      current: info.current,
      target: info.wanted,
      type: 'SEMVER_LOCKFILE_DRIFT',
      message: `Installed version (${info.current}) differs from wanted version (${info.wanted}) in package.json semver range.`
    });
    continue;
  }

  // Rule 2: Upstream freshness vs approved compatibility pin
  const pinRule = approvedPins[pkg];
  if (pinRule) {
    const currentMajor = info.current.split('.')[0];
    const allowedMajor = pinRule.track ? pinRule.track.replace('^', '').split('.')[0] : pinRule.major;
    if (allowedMajor && currentMajor === String(allowedMajor)) {
      console.log(`  ✓ ${pkg}@${info.current} (pinned track ${pinRule.track || allowedMajor + '.x'}: ${pinRule.reason || 'compatibility'})`);
      continue;
    }
  }

  // Check allowOutdated list
  const isAllowed = (frontendPolicy.allowOutdated || []).includes(pkg);
  if (isAllowed) {
    console.log(`  ✓ ${pkg}@${info.current} (explicitly allowed outdated)`);
    continue;
  }

  // If not pinned or allowed, it requires attention!
  violations.push({
    pkg,
    current: info.current,
    target: info.latest,
    type: 'UNREVIEWED_UPSTREAM',
    message: `Package has newer upstream release (${info.latest}) not covered by approved compatibility policy.`
  });
}

// Log non-outdated packages as fresh
for (const [pkg, ver] of Object.entries(allDeps)) {
  if (!outdated[pkg]) {
    console.log(`  ✓ ${pkg}@${ver} (latest)`);
  }
}

if (violations.length > 0) {
  console.error('\n❌ Frontend Dependency Freshness Policy Violation:');
  console.error('The following dependencies require attention:');
  for (const v of violations) {
    console.error(`  - ${v.pkg}: current ${v.current} -> required ${v.target} [${v.type}]`);
    console.error(`    ${v.message}`);
  }
  console.error('\nAction required: Run "npm update", review compatibility, or update .dependency-policy.json.');
  process.exit(1);
}

console.log('✅ All frontend dependencies satisfy freshness and compatibility policy.');
