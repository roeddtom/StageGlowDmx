import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { emptyConfig, makeTemplate, makeMood, removeMood, tapBPM, validateConfig } from './model.ts'

function patchedRig() {
  const config = emptyConfig()
  const profile = makeTemplate('RGB par', ['red', 'green', 'blue'])
  const group = { id: 'front', name: 'Front' }
  const fixture = { id: 'par-1', name: 'Front left', profileId: profile.id, startAddress: 1, groupId: group.id }
  const loadout = { id: 'gig', name: 'Gig', fixtures: [fixture] }
  return { ...config, profiles: [profile], groups: [group], loadouts: [loadout], activeLoadoutId: loadout.id }
}

describe('configuration validation', () => {
  it('validates show slots and pause references while accepting old backups', () => {
    const config = patchedRig()
    const mood = makeMood(config.groups)
    config.moods = [mood]
    for (const count of [8, 10, 12]) {
      config.show = { slots: Array(count).fill(mood.id), pauseMoodId: mood.id }
      assert.deepEqual(validateConfig(config), [])
    }
    for (const show of [null, {}, { slots: [], pauseMoodId: '' },
      { slots: Array(13).fill(''), pauseMoodId: '' }, { slots: Array(8).fill(null), pauseMoodId: '' },
      { slots: Array(8).fill('missing'), pauseMoodId: '' }, { slots: Array(8).fill(''), pauseMoodId: 'missing' },
      { slots: Array(8).fill(''), pauseMoodId: null }, { slots: Array(8).fill(''), pauseMoodId: '', extra: 1 }]) {
      config.show = show
      assert.ok(validateConfig(config).length > 0)
    }
    delete config.show
    assert.deepEqual(validateConfig(config), [])
  })

  it('clears every show and pause assignment when deleting a mood without mutating config', () => {
    const config = patchedRig()
    const first = makeMood(config.groups), second = makeMood(config.groups)
    config.moods = [first, second]
    config.show.slots[0] = first.id
    config.show.slots[1] = second.id
    config.show.slots[2] = first.id
    config.show.pauseMoodId = first.id
    const next = removeMood(config, first.id)
    assert.deepEqual(validateConfig(next), [])
    assert.deepEqual(next.show.slots.slice(0, 3), ['', second.id, ''])
    assert.equal(next.show.pauseMoodId, '')
    assert.equal(config.show.pauseMoodId, first.id)
    assert.equal(config.moods.length, 2)
  })

  it('accepts legacy output defaults and only safe machine-local serial paths', () => {
    const config = patchedRig()
    delete config.output
    assert.deepEqual(validateConfig(config), [])
    for (const serialPort of ['', 'COM3', '/dev/ttyUSB0', '/dev/ttyACM1', '/dev/serial/by-id/usb-ENTTEC_123-if00-port0']) {
      config.output = { mode: 'enttec', serialPort }
      assert.deepEqual(validateConfig(config), [])
    }
    for (const output of [null, {}, { mode: 'raw', serialPort: '' }, { mode: 'enttec', serialPort: '/etc/passwd' },
      { mode: 'enttec', serialPort: '/dev/serial/by-id/..' }, { mode: 'enttec', serialPort: 'COM0' },
      { mode: 'enttec', serialPort: '', enabled: true }]) {
      config.output = output
      assert.ok(validateConfig(config).length > 0)
    }
  })

  it('accepts a valid profile and fixture patch', () => {
    assert.deepEqual(validateConfig(patchedRig()), [])
  })

  describe('moods and tempo', () => {
    it('builds a starter with steady front, evolving back and UV off', () => {
      const config = patchedRig()
      config.groups.push({ id: 'back', name: 'Back' })
      const mood = makeMood(config.groups, true)
      config.moods = [mood]
      assert.deepEqual(validateConfig(config), [])
      assert.equal(mood.groups[0].colorMode, 'static')
      assert.equal(mood.groups[0].intensityMin, mood.groups[0].intensityMax)
      assert.equal(mood.groups[1].colorMode, 'palette')
      assert.ok(mood.groups.every((group) => group.uv === 0))
    })

    it('rejects invalid palettes, ranges, modes, timing and references', () => {
      for (const edit of [
        (mood) => { mood.groups[0].palette = [] },
        (mood) => { mood.groups[0].palette = ['red'] },
        (mood) => { mood.groups[0].intensityMin = 0.9 },
        (mood) => { mood.groups[0].colorMode = 'strobe' },
        (mood) => { mood.groups[0].groupId = 'deleted' },
        (mood) => { mood.groups[0].uv = 1.1 },
        (mood) => { mood.groups.push(structuredClone(mood.groups[0])) },
        (mood) => { mood.smoothness = 0 },
        (mood) => { mood.beatsPerChange = NaN },
        (mood) => { mood.crossfadeSeconds = 31 },
      ]) {
        const config = patchedRig()
        config.moods = [makeMood(config.groups)]
        edit(config.moods[0])
        assert.ok(validateConfig(config).length > 0)
      }
    })

    it('accepts old schema-1 backups without moods and rejects explicit null', () => {
      const config = patchedRig()
      delete config.moods
      assert.deepEqual(validateConfig(config), [])
      config.moods = null
      assert.match(validateConfig(config).join(' '), /Moods must be an array/)
    })

    it('calculates tapped tempo, rejects bounce, and resets after a long pause', () => {
      let result = tapBPM([], 1000)
      assert.equal(result.bpm, undefined)
      result = tapBPM(result.taps, 1500)
      assert.equal(result.bpm, 120)
      assert.deepEqual(tapBPM(result.taps, 1550).taps, result.taps)
      result = tapBPM(result.taps, 2000)
      assert.equal(result.bpm, 120)
      result = tapBPM(result.taps, 6000)
      assert.deepEqual(result, { taps: [6000] })
    })
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
