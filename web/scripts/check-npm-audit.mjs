// Dependency audit for CI.
//
// Production dependencies (what ships in the frontend) must have no advisory
// of moderate severity or above. Development dependencies must not either,
// except advisories listed in audit-allowlist.json, each with a reason and an
// expiry date; an expired or unused entry fails the check.
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)))
const levels = { info: 0, low: 1, moderate: 2, high: 3, critical: 4 }
const threshold = levels.moderate

function audit(args) {
  let out
  try {
    out = execFileSync('npm', ['audit', '--json', ...args], { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] })
  } catch (error) {
    // npm audit exits non-zero when it finds advisories; the JSON is still on stdout.
    out = error.stdout
  }
  const report = JSON.parse(out)
  if (report.error) throw new Error(`npm audit failed: ${JSON.stringify(report.error)}`)
  return report
}

// advisories returns the advisories at or above the threshold, keyed by GHSA id.
export function advisories(report) {
  const found = new Map()
  for (const [name, vuln] of Object.entries(report.vulnerabilities || {})) {
    for (const via of vuln.via || []) {
      if (typeof via !== 'object' || (levels[via.severity] ?? 0) < threshold) continue
      const id = String(via.url || '').split('/').pop() || `${name}:${via.source}`
      found.set(id, { id, package: via.name || name, severity: via.severity, title: via.title })
    }
  }
  return found
}

export function evaluate(prod, all, allowlist, today) {
  const errors = []
  for (const adv of prod.values()) {
    errors.push(`${adv.id} (${adv.package}, ${adv.severity}) affects production dependencies: ${adv.title}`)
  }
  const waived = new Map((allowlist.advisories || []).map(entry => [entry.id, entry]))
  for (const adv of all.values()) {
    if (prod.has(adv.id)) continue
    const entry = waived.get(adv.id)
    if (!entry) {
      errors.push(`${adv.id} (${adv.package}, ${adv.severity}) affects development dependencies and is not in audit-allowlist.json: ${adv.title}`)
    } else if (!(entry.expires >= today)) {
      errors.push(`${adv.id} waiver expired on ${entry.expires}; re-check for a fix or renew it with the owner's approval`)
    }
  }
  for (const entry of waived.values()) {
    if (!all.has(entry.id)) errors.push(`${entry.id} is in audit-allowlist.json but no longer reported; remove the entry`)
  }
  return errors
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const allowlist = JSON.parse(readFileSync(path.join(root, 'audit-allowlist.json'), 'utf8'))
  const today = new Date().toISOString().slice(0, 10)
  const prod = advisories(audit(['--omit=dev']))
  const all = advisories(audit([]))
  const errors = evaluate(prod, all, allowlist, today)
  for (const entry of allowlist.advisories || []) {
    if (all.has(entry.id) && !prod.has(entry.id) && entry.expires >= today) {
      console.log(`waived until ${entry.expires}: ${entry.id} (${entry.package}, development only)`)
    }
  }
  if (errors.length) {
    for (const error of errors) console.error(`npm audit: ${error}`)
    process.exit(1)
  }
  console.log(`npm audit: ok (${prod.size} production, ${all.size} total advisories at moderate or above)`)
}
