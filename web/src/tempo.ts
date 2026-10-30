export type TempoFocus = 'kick' | 'snare' | 'full'

export function normalizeTempo(bpm: number): number | undefined {
  if (!Number.isFinite(bpm) || bpm <= 0) return
  while (bpm < 50) bpm *= 2
  while (bpm > 180) bpm /= 2
  return Math.round(bpm)
}

// Missing beats and half/double-time rhythms are ambiguous: prefer the current tempo.
export function estimateTempo(onsets: number[], reference: number): number | undefined {
  if (onsets.length < 6 || onsets.at(-1)! - onsets[0] < 2500) return
  const pairs = onsets.flatMap((time, i) => onsets.slice(i + 1).map((next) => next - time))
  const candidates = new Map<number, number>()
  for (const interval of pairs) {
    for (const beats of [1, 2, 3, 4]) {
      const bpm = 60000 * beats / interval
      if (Number.isFinite(bpm) && bpm >= 25 && bpm <= 500) candidates.set(Math.round(bpm * 2), bpm)
    }
  }
  let best: number | undefined
  let bestScore = -Infinity
  for (const bpm of candidates.values()) {
    const period = 60000 / bpm
    let phaseX = 0
    let phaseY = 0
    for (const time of onsets) {
      const phase = (time - onsets[0]) / period * 2 * Math.PI
      phaseX += Math.cos(phase)
      phaseY += Math.sin(phase)
    }
    const coherence = Math.hypot(phaseX, phaseY) / onsets.length
    if (coherence < 0.75) continue
    let supported = 0
    let matches = 0
    let error = 0
    for (const time of onsets) {
      let neighbours = 0
      for (const other of onsets) {
        const beats = Math.abs(other - time) / period
        const distance = Math.abs(beats - Math.round(beats))
        if (beats >= 0.8 && beats <= 4.2 && distance < 0.1) {
          neighbours++
          error += distance
        }
      }
      if (neighbours >= 2) supported++
      matches += neighbours
    }
    if (supported / onsets.length < 0.7 || matches < onsets.length * 2) continue
    const score = coherence + supported / onsets.length - error / matches -
      0.12 * Math.abs(Math.log2(bpm / (Number.isFinite(reference) && reference > 0 ? reference : 120)))
    if (score > bestScore) { best = bpm; bestScore = score }
  }
  if (best === undefined) return
  let totalTime = 0
  let totalBeats = 0
  for (const interval of pairs) {
    const beats = interval * best / 60000
    const rounded = Math.round(beats)
    if (rounded >= 1 && rounded <= 4 && Math.abs(beats - rounded) < 0.1) {
      totalTime += interval
      totalBeats += rounded
    }
  }
  return normalizeTempo(Math.round(60000 * totalBeats / totalTime))
}

export class TempoDetector {
  private history: { time: number; energy: number }[] = []
  private onsets: number[] = []
  private lastOnset = -Infinity
  private previous = 0
  private beforePrevious = 0
  private previousTime = 0
  private candidate = 0
  private confirmations = 0
  private lastTime = -Infinity

  get lastBeatTime(): number { return this.lastOnset }

  sample(time: number, energy: number, reference: number): number | undefined {
    if (!Number.isFinite(time) || !Number.isFinite(energy) || energy < 0) return
    if (time <= this.lastTime || time - this.lastTime > 2000) {
      this.history = []
      this.onsets = []
      this.previous = 0
      this.beforePrevious = 0
      this.lastOnset = -Infinity
      this.confirmations = 0
    }
    this.lastTime = time
    this.history = this.history.filter((sample) => time - sample.time < 1000)
    const baseline = this.history.reduce((sum, sample) => sum + sample.energy, 0) / Math.max(1, this.history.length)
    const deviation = Math.sqrt(this.history.reduce((sum, sample) => sum + (sample.energy - baseline) ** 2, 0) / Math.max(1, this.history.length))
    const onset = this.history.length >= 10 && this.previous > Math.max(0.003, baseline + deviation * 0.8) &&
      this.previous > this.beforePrevious && this.previous >= energy && this.previousTime - this.lastOnset >= 120
    this.history.push({ time, energy })
    this.beforePrevious = this.previous
    this.previous = energy
    const onsetTime = this.previousTime
    this.previousTime = time
    if (time - this.lastOnset > 4000) { this.onsets = []; this.confirmations = 0 }
    if (!onset) return
    this.lastOnset = onsetTime
    this.onsets = [...this.onsets.filter((onset) => time - onset < 16000), onsetTime].slice(-48)
    const bpm = estimateTempo(this.onsets, reference)
    if (bpm === undefined) { this.confirmations = 0; return }
    this.confirmations = Math.abs(bpm - this.candidate) <= 3 ? this.confirmations + 1 : 1
    this.candidate = bpm
    return this.confirmations >= 3 ? bpm : undefined
  }
}

export function signalStrength(samples: Float32Array): number {
  const power = samples.reduce((sum, value) => sum + value * value, 0) / Math.max(1, samples.length)
  if (!Number.isFinite(power) || power <= 0) return 0
  return Math.round(Math.max(0, Math.min(1, (10 * Math.log10(power) + 60) / 48)) * 100)
}

export class SpectralFlux {
  private previous: number[] = []

  sample(spectrum: Float32Array, sampleRate: number, fftSize: number, focus: TempoFocus): number {
    const [from, to] = bandBins(spectrum.length, sampleRate, fftSize, focus)
    let flux = 0
    for (let bin = from; bin <= to; bin++) {
      const magnitude = Math.log1p(10 ** (spectrum[bin] / 20) / 0.001)
      // Positive spectral changes reveal attacks even over sustained instruments.
      if (this.previous[bin] !== undefined) flux += Math.max(0, magnitude - this.previous[bin])
      this.previous[bin] = magnitude
    }
    return flux / Math.max(1, to - from + 1)
  }
}

function bandBins(length: number, sampleRate: number, fftSize: number, focus: TempoFocus): [number, number] {
  const [low, high] = focus === 'kick' ? [40, 180] : focus === 'snare' ? [700, 5000] : [40, 5000]
  return [Math.max(1, Math.ceil(low * fftSize / sampleRate)), Math.min(length - 1, Math.floor(high * fftSize / sampleRate))]
}
