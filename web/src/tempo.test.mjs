import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { estimateTempo, normalizeTempo, TempoDetector, bandEnergy } from './tempo.ts'

describe('microphone tempo', () => {
  it('estimates bounded rhythms despite jitter and missed beats', () => {
    for (const bpm of [50, 60, 90, 120, 160, 180]) {
      const period = 60000 / bpm
      const onsets = Array.from({ length: 14 }, (_, i) => 500 + i * period + (i % 2 ? 3 : -3))
      onsets.splice(6, 1)
      const result = estimateTempo(onsets, bpm)
      assert.ok(Math.abs(result - bpm) <= 3, `${bpm} -> ${result}`)
      assert.ok(result >= 50 && result <= 180)
    }
  })

  it('folds slow and fast estimates into 50-180 BPM rather than clamping', () => {
    for (const [raw, expected] of [[25, 50], [40, 80], [49, 98], [50, 50], [180, 180], [181, 91], [200, 100], [210, 105], [360, 180], [400, 100]]) {
      assert.equal(normalizeTempo(raw), expected)
      const onsets = Array.from({ length: 24 }, (_, i) => i * 60000 / raw)
      assert.equal(estimateTempo(onsets, raw), expected, `estimated ${raw}`)
    }
    for (const invalid of [0, -1, NaN, Infinity]) assert.equal(normalizeTempo(invalid), undefined)
  })

  it('folds out-of-range pulse trains through onset detection', () => {
    for (const [raw, expected] of [[40, 80], [200, 100], [300, 150]]) {
      const detector = new TempoDetector()
      let result
      for (let time = 0; time <= 24000; time += 20) {
        result = detector.sample(time, time % (60000 / raw) < 40 ? 0.1 : 0.001, raw) ?? result
      }
      assert.equal(result, expected, `pulse train ${raw}`)
    }
  })

  it('holds tempo rather than guessing from sparse or irregular transients', () => {
    assert.equal(estimateTempo([0, 500, 1000], 120), undefined)
    assert.equal(estimateTempo([0, 311, 1007, 1473, 2419, 2791, 3593, 4057], 120), undefined)
  })

  it('uses the existing tempo to resolve half-time and double-time ambiguity', () => {
    const onsets = Array.from({ length: 12 }, (_, i) => i * 1000)
    assert.equal(estimateTempo(onsets, 60), 60)
    assert.equal(estimateTempo(onsets, 120), 120)
  })

  it('detects sustained pulse trains, ignores steady noise, and reacquires after silence', () => {
    const detector = new TempoDetector()
    let result
    for (let time = 0; time <= 10000; time += 20) {
      result = detector.sample(time, time % 500 < 40 ? 0.1 : 0.001, 120) ?? result
    }
    assert.equal(result, 120)
    for (let time = 10020; time < 16000; time += 20) {
      assert.equal(detector.sample(time, 0.001, 120), undefined)
    }
    let reacquired
    for (let time = 16000; time <= 26000; time += 20) {
      reacquired = detector.sample(time, (time - 16000) % 400 < 40 ? 0.1 : 0.001, 150) ?? reacquired
    }
    assert.equal(reacquired, 150)
    const steady = new TempoDetector()
    for (let time = 0; time < 10000; time += 20) assert.equal(steady.sample(time, 0.05, 120), undefined)
  })

  it('rejects invalid audio values and resets after a browser timing gap', () => {
    const detector = new TempoDetector()
    assert.equal(detector.sample(0, NaN, 120), undefined)
    assert.equal(detector.sample(0, -1, 120), undefined)
    for (let time = 0; time < 10000; time += 20) detector.sample(time, time % 500 < 40 ? 0.1 : 0.001, 120)
    assert.equal(detector.sample(30000, 0.1, 120), undefined)
  })

  it('separates kick and snare frequency bands', () => {
    const spectrum = new Float32Array(1024).fill(-100)
    spectrum[4] = -12 // 94 Hz at 48 kHz / 2048.
    assert.ok(bandEnergy(spectrum, 48000, 2048, 'kick') > bandEnergy(spectrum, 48000, 2048, 'snare') * 100)
    spectrum.fill(-100)
    spectrum[80] = -12
    assert.ok(bandEnergy(spectrum, 48000, 2048, 'snare') > bandEnergy(spectrum, 48000, 2048, 'kick') * 100)
  })
})
