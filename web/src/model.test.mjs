import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { emptyConfig, makeTemplate, validateConfig } from './model.ts'

function patchedRig() {
  const config = emptyConfig()
  const profile = makeTemplate('RGB par', ['red', 'green', 'blue'])
  const group = { id: 'front', name: 'Front' }
  const fixture = { id: 'par-1', name: 'Front left', profileId: profile.id, startAddress: 1, groupId: group.id }
  const loadout = { id: 'gig', name: 'Gig', fixtures: [fixture] }
  return { ...config, profiles: [profile], groups: [group], loadouts: [loadout], activeLoadoutId: loadout.id }
}

describe('configuration validation', () => {
  it('accepts a valid profile and fixture patch', () => {
    assert.deepEqual(validateConfig(patchedRig()), [])
  })

  it('accepts finite fractional mix and tempo settings in the backend ranges', () => {
    const config = patchedRig()
    config.profiles[0].whiteMix = 0.45
    config.profiles[0].amberMix = 0.8
    config.bpm = 120.5
    assert.deepEqual(validateConfig(config), [])
  })

  it('requires a complete profile map, RGB, and safe values for fixed channels', () => {
    const config = patchedRig()
    const profile = config.profiles[0]
    profile.channels[0] = { offset: 1, kind: 'fixed' }
    profile.channels.push({ offset: 2, kind: 'fixed' })
    const errors = validateConfig(config).join(' ')
    assert.match(errors, /fixed channel at offset 2 needs a safe value/)
    assert.match(errors, /needs exactly one red channel/)
    assert.match(errors, /repeats offset 1/)
  })

  it('rejects overlapping fixtures and patches extending past address 512', () => {
    const config = patchedRig()
    config.loadouts[0].fixtures.push({
      id: 'par-2', name: 'Front right', profileId: config.profiles[0].id, startAddress: 3, groupId: 'front',
    })
    assert.match(validateConfig(config).join(' '), /overlapping fixtures at address 3/)
    config.loadouts[0].fixtures[1].startAddress = 511
    assert.match(validateConfig(config).join(' '), /extends past DMX address 512/)
  })

  it('rejects broken references, non-unicast destinations, and invalid settings', () => {
    const config = patchedRig()
    config.loadouts[0].fixtures[0].groupId = 'missing'
    config.artNet.destinationIp = '239.1.2.3'
    config.bpm = 301
    const errors = validateConfig(config).join(' ')
    assert.match(errors, /existing group/)
    assert.match(errors, /unicast IPv4/)
    assert.match(errors, /BPM must be between 20 and 300/)
  })

  it('reports malformed records and unsupported schema fields without throwing', () => {
    const config = patchedRig()
    config.surprise = true
    config.profiles.push(null)
    assert.doesNotThrow(() => validateConfig(config))
    const errors = validateConfig(config).join(' ')
    assert.match(errors, /unsupported field "surprise"/)
    assert.match(errors, /Profile must be a JSON object/)
  })
})
