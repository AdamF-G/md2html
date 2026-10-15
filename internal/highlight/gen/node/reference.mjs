import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import vsctm from 'vscode-textmate'
import oniguruma from 'vscode-oniguruma'

const require = createRequire(import.meta.url)
const work = process.argv[2]
const selected = JSON.parse(fs.readFileSync(path.join(work, 'selected.json'), 'utf8'))
// The collection exports its grammars by path but not its package.json.
const byScope = new Map(selected.map(s => [s.scope, require.resolve(`tm-grammars/grammars/${s.name}.json`)]))

const wasm = fs.readFileSync(require.resolve('vscode-oniguruma/release/onig.wasm')).buffer
await oniguruma.loadWASM(wasm)
const registry = new vsctm.Registry({
  onigLib: Promise.resolve({
    createOnigScanner: p => new oniguruma.OnigScanner(p),
    createOnigString: s => new oniguruma.OnigString(s),
  }),
  loadGrammar: async scope => {
    const file = byScope.get(scope)
    return file ? vsctm.parseRawGrammar(fs.readFileSync(file, 'utf8'), file) : null
  },
})

fs.mkdirSync(path.join(work, 'ref'), { recursive: true })
for (const s of selected) {
  if (!s.sample) continue
  const grammar = await registry.loadGrammar(s.scope)
  const lines = fs.readFileSync(s.sample, 'utf8').replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n')
  let state = vsctm.INITIAL
  const tokens = []
  for (const line of lines) {
    const r = grammar.tokenizeLine(line, state, 0)
    tokens.push(r.tokens.map(t => ({ s: t.startIndex, e: Math.min(t.endIndex, line.length), x: t.scopes.join(' ') })))
    state = r.ruleStack
  }
  fs.writeFileSync(path.join(work, 'ref', s.name + '.json'),
    JSON.stringify({ language: s.name, scope: s.scope, lines, tokens }))
}
