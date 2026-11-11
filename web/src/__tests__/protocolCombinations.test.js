import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { PROTOCOL_CAPABILITIES, transportsFor } from '@/views/admin/nodes/protocolCapabilities'
import { PROTOCOL_TYPES } from '@/views/admin/nodes/useProtocolEditor'

// The protocol, TLS mode and transport combinations the Agent's kernel runs.
// anix-agent holds the same file and maps each of them in its own test, and
// Control's saving is held to it by TestProtocolCombinationsMatchTheKernel.
const table = JSON.parse(readFileSync(path.resolve(__dirname, '../../../contracts/agent/v1/protocol-combinations.json'), 'utf8'))
const listed = new Set(table.combinations.flatMap(row => row.tls.flatMap(tls => row.transports.map(transport => `${row.type}/${tls}/${transport}`))))

describe('node protocol combinations', () => {
  it('offers only what the kernel runs', () => {
    const offered = PROTOCOL_TYPES.flatMap(type => PROTOCOL_CAPABILITIES[type].tls.flatMap(tls => transportsFor(type, tls).map(transport => `${type}/${tls}/${transport}`)))
    expect(offered.filter(combination => !listed.has(combination))).toEqual([])
  })
})
