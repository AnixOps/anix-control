#!/usr/bin/env node
// Vendors AnixOps Design (github.com/AnixOps/AnixOps-design) into
// web/src/design/ and verifies the vendored copy.
//
//   node scripts/sync-design.mjs --tag v1.0.2                  # fetch from GitHub
//   node scripts/sync-design.mjs --tag v1.0.2 --source ../AnixOps-design
//   node scripts/sync-design.mjs --check                       # offline, for CI
//   node scripts/sync-design.mjs --check --upstream            # also compare with GitHub
//
// --source accepts a git checkout (files are read from the tag with
// `git show <tag>:<path>`, so the working tree state does not matter) or a
// plain directory laid out like the repository.
//
// --check needs no network: it hashes the vendored files and compares them
// with web/src/design/manifest.json, which the sync wrote from the tag.
// Never edit a vendored file by hand; change the design repository, tag it,
// and sync. See docs/reference/frontend-design.md.

import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

export const REPOSITORY = 'AnixOps/AnixOps-design'
export const DESIGN_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src/design')

// Vendored path (relative to web/src/design) -> path in the design repository.
export const FILES = {
  'tokens.css': 'tokens/css/tokens.css',
  'brand/mark-glyph.svg': 'brand/assets/mark-glyph.svg',
  'brand/wordmark.svg': 'brand/assets/wordmark.svg',
  'brand/wordmark-on-dark.svg': 'brand/assets/wordmark-on-dark.svg',
  'brand/favicon.svg': 'brand/assets/favicon.svg',
  'brand/favicon/favicon.ico': 'brand/assets/favicon/favicon.ico',
  'brand/favicon/apple-touch-icon.png': 'brand/assets/favicon/apple-touch-icon.png',
  'brand/favicon/icon-192.png': 'brand/assets/favicon/icon-192.png',
  'brand/favicon/icon-512.png': 'brand/assets/favicon/icon-512.png',
  'brand/favicon/icon-maskable-512.png': 'brand/assets/favicon/icon-maskable-512.png',
  'fonts/inter/InterVariable-latin.woff2': 'fonts/inter/InterVariable-latin.woff2',
  'fonts/inter/inter.css': 'fonts/inter/inter.css',
  'fonts/inter/OFL.txt': 'fonts/inter/OFL.txt',
  LICENSE: 'LICENSE',
  'LICENSE-BRAND.md': 'LICENSE-BRAND.md'
}

// Files this script writes itself; --check regenerates and compares them.
const GENERATED = ['VERSION', 'NOTICE.md', 'manifest.json']
const TAG_PATTERN = /^v\d+\.\d+\.\d+$/

export function sha256(buffer) {
  return createHash('sha256').update(buffer).digest('hex')
}

export function noticeText(tag) {
  return `# AnixOps Design ${tag} (vendored)

The files in this directory are copied unmodified from
https://github.com/${REPOSITORY} at tag \`${tag}\` by
\`web/scripts/sync-design.mjs\`. Do not edit them here: change the design
repository, tag a release, then run \`npm run design:sync -- --tag <tag>\`.
\`npm run design:check\` (CI) verifies them against \`manifest.json\`.

Licences:

- \`tokens.css\`: MIT, see \`LICENSE\`.
- \`fonts/inter/\`: Inter, SIL Open Font License 1.1, see \`fonts/inter/OFL.txt\`.
- \`brand/\` (the AnixOps mark, glyph, wordmarks, favicon and app icons) and the
  name "AnixOps": **not** MIT. All rights reserved; use is limited to official
  AnixOps products, see \`LICENSE-BRAND.md\`. Forks and redistributions that are
  not official AnixOps releases must replace these assets with their own.
`
}

function manifestFor(tag, files) {
  return {
    name: 'AnixOps Design',
    repository: `https://github.com/${REPOSITORY}`,
    tag,
    files: Object.keys(FILES).sort().map(dest => ({
      path: dest,
      source: FILES[dest],
      bytes: files[dest].length,
      sha256: sha256(files[dest])
    }))
  }
}

function serialize(value) {
  return `${JSON.stringify(value, null, 2)}\n`
}

async function fetchFromGitHub(tag, source) {
  const url = `https://raw.githubusercontent.com/${REPOSITORY}/${encodeURIComponent(tag)}/${source}`
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(`GET ${url}: HTTP ${response.status}`)
  }
  return Buffer.from(await response.arrayBuffer())
}

function readFromSource(sourceRoot, tag, source) {
  if (existsSync(path.join(sourceRoot, '.git'))) {
    return execFileSync('git', ['-C', sourceRoot, 'show', `${tag}:${source}`], {
      maxBuffer: 64 * 1024 * 1024
    })
  }
  return readFileSync(path.join(sourceRoot, source))
}

export async function collect(tag, sourceRoot) {
  const files = {}
  for (const [dest, source] of Object.entries(FILES)) {
    files[dest] = sourceRoot ? readFromSource(sourceRoot, tag, source) : await fetchFromGitHub(tag, source)
  }
  // The generated tokens.css names the design version in its header; refuse a
  // source whose content does not belong to the requested tag.
  const header = files['tokens.css'].toString('utf8').split('\n', 1)[0]
  if (!header.includes(`AnixOps Design ${tag.slice(1)}.`)) {
    throw new Error(`tokens.css header does not match ${tag}: ${header}`)
  }
  return files
}

export function write(tag, files, designDir = DESIGN_DIR) {
  for (const [dest, content] of Object.entries(files)) {
    const target = path.join(designDir, dest)
    mkdirSync(path.dirname(target), { recursive: true })
    writeFileSync(target, content)
  }
  writeFileSync(path.join(designDir, 'VERSION'), `${tag}\n`)
  writeFileSync(path.join(designDir, 'NOTICE.md'), noticeText(tag))
  writeFileSync(path.join(designDir, 'manifest.json'), serialize(manifestFor(tag, files)))
}

function listFiles(dir, prefix = '') {
  return readdirSync(dir).flatMap(name => {
    const relative = prefix ? `${prefix}/${name}` : name
    return statSync(path.join(dir, name)).isDirectory() ? listFiles(path.join(dir, name), relative) : [relative]
  })
}

// check returns a list of problems; an empty list means the vendored copy is
// exactly the pinned tag.
export function check(designDir = DESIGN_DIR) {
  const problems = []
  const manifestPath = path.join(designDir, 'manifest.json')
  if (!existsSync(manifestPath)) {
    return [`${manifestPath} is missing; run npm run design:sync -- --tag <tag>`]
  }
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
  const tag = manifest.tag
  if (!TAG_PATTERN.test(tag || '')) {
    problems.push(`manifest.json: invalid tag ${JSON.stringify(tag)}`)
  }

  const version = existsSync(path.join(designDir, 'VERSION'))
    ? readFileSync(path.join(designDir, 'VERSION'), 'utf8')
    : ''
  if (version !== `${tag}\n`) {
    problems.push(`VERSION is ${JSON.stringify(version.trim())}, manifest pins ${tag}`)
  }
  const notice = existsSync(path.join(designDir, 'NOTICE.md'))
    ? readFileSync(path.join(designDir, 'NOTICE.md'), 'utf8')
    : ''
  if (notice !== noticeText(tag)) {
    problems.push('NOTICE.md differs from the generated notice; rerun the sync')
  }

  const listed = new Map((manifest.files || []).map(entry => [entry.path, entry]))
  const expected = Object.keys(FILES).sort()
  for (const dest of expected) {
    const entry = listed.get(dest)
    if (!entry) {
      problems.push(`${dest}: in sync-design.mjs FILES but not in manifest.json; rerun the sync`)
      continue
    }
    if (entry.source !== FILES[dest]) {
      problems.push(`${dest}: manifest source ${entry.source} differs from FILES (${FILES[dest]})`)
    }
    const target = path.join(designDir, dest)
    if (!existsSync(target)) {
      problems.push(`${dest}: missing`)
      continue
    }
    const content = readFileSync(target)
    if (content.length !== entry.bytes || sha256(content) !== entry.sha256) {
      problems.push(`${dest}: differs from ${tag} (sha256 ${sha256(content)}, expected ${entry.sha256}); vendored files must not be edited`)
    }
  }
  for (const dest of listed.keys()) {
    if (!(dest in FILES)) {
      problems.push(`${dest}: in manifest.json but not in sync-design.mjs FILES`)
    }
  }
  for (const file of listFiles(designDir)) {
    if (!(file in FILES) && !GENERATED.includes(file)) {
      problems.push(`${file}: unexpected file in the vendored directory`)
    }
  }
  return problems
}

export async function checkUpstream(designDir = DESIGN_DIR, sourceRoot) {
  const manifest = JSON.parse(readFileSync(path.join(designDir, 'manifest.json'), 'utf8'))
  const files = await collect(manifest.tag, sourceRoot)
  return manifest.files
    .filter(entry => sha256(files[entry.path]) !== entry.sha256)
    .map(entry => `${entry.path}: manifest.json does not match ${entry.source} at ${manifest.tag} upstream`)
}

function parseArgs(argv) {
  const options = { check: false, upstream: false, tag: '', source: '' }
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index]
    if (arg === '--check') options.check = true
    else if (arg === '--upstream') options.upstream = true
    else if (arg === '--tag') options.tag = argv[++index] || ''
    else if (arg === '--source') options.source = argv[++index] || ''
    else if (arg === '-h' || arg === '--help') options.help = true
    else throw new Error(`unknown argument ${arg}`)
  }
  return options
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  if (options.help) {
    console.log('usage: sync-design.mjs --tag vX.Y.Z [--source <design repo or dir>] | --check [--upstream [--source <dir>]]')
    return 0
  }
  const sourceRoot = options.source ? path.resolve(options.source) : ''

  if (options.check) {
    const problems = check()
    if (options.upstream && problems.length === 0) {
      problems.push(...await checkUpstream(DESIGN_DIR, sourceRoot))
    }
    const tag = readFileSync(path.join(DESIGN_DIR, 'VERSION'), 'utf8').trim()
    if (problems.length > 0) {
      console.error(`AnixOps Design check failed (${problems.length}):`)
      for (const problem of problems) console.error(`  - ${problem}`)
      return 1
    }
    console.log(`AnixOps Design ${tag}: ${Object.keys(FILES).length} vendored files match manifest.json${options.upstream ? ' and upstream' : ''}`)
    return 0
  }

  if (!TAG_PATTERN.test(options.tag)) {
    throw new Error('--tag vX.Y.Z is required (or use --check)')
  }
  const files = await collect(options.tag, sourceRoot)
  write(options.tag, files)
  console.log(`Vendored AnixOps Design ${options.tag} from ${sourceRoot || `github.com/${REPOSITORY}`} into ${path.relative(process.cwd(), DESIGN_DIR) || '.'}`)
  return 0
}

if (import.meta.url === pathToFileURL(process.argv[1] || '').href) {
  main().then(code => process.exit(code), error => {
    console.error(error.message)
    process.exit(1)
  })
}
