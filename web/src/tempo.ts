export type TempoFocus = 'kick' | 'snare' | 'full'

// Missing beats and half/double-time rhythms are ambiguous: prefer the current tempo.
export function estimateTempo(onsets: number[], reference: number): number | undefined {
  const intervals = onsets.slice(1).map((time, i) => time - onsets[i])
  if (intervals.length < 5 || onsets.at(-1)! - onsets[0] < 2500) return
  const candidates = intervals.flatMap((interval) => [0.5, 1, 2, 3, 4].map((beats) => 60000 * beats / interval))
    .filter((bpm) => bpm >= 50 && bpm <= 210)
  let best: number | undefined
  let bestScore = -Infinity
  for (const bpm of candidates) {
    const period = 60000 / bpm
    let matches = 0
    let error = 0
    for (const interval of intervals) {
      const beats = interval / period
      const distance = Math.abs(beats - Math.round(beats))
      if (beats >= 0.8 && beats <= 4.2 && distance < 0.12) matches++
      error += Math.min(distance, 0.3)
    }
    if (matches / intervals.length < 0.8) continue
    const score = matches / intervals.length - error / intervals.length -
      0.12 * Math.abs(Math.log2(bpm / Math.max(50, Math.min(210, reference))))
    if (score > bestScore) { best = bpm; bestScore = score }
  }
  if (best === undefined) return
  let totalTime = 0
  let totalBeats = 0
  for (const interval of intervals) {
    const beats = interval * best / 60000
    const rounded = Math.round(beats)
    if (rounded >= 1 && rounded <= 4 && Math.abs(beats - rounded) < 0.12) {
      totalTime += interval
      totalBeats += rounded
    }
  }
  return Math.max(50, Math.min(210, Math.round(60000 * totalBeats / totalTime)))
}

export class TempoDetector {
  private history: { time: number; energy: number }[] = []
  private onsets: number[] = []
  private lastOnset = -Infinity
  private previous = 0
  private candidate = 0
  private confirmations = 0
  private lastTime = -Infinity

  sample(time: number, energy: number, reference: number): number | undefined {
    if (!Number.isFinite(time) || !Number.isFinite(energy) || energy < 0) return
    if (time <= this.lastTime || time - this.lastTime > 2000) {
      this.history = []
      this.onsets = []
      this.previous = 0
      this.lastOnset = -Infinity
      this.confirmations = 0
    }
    this.lastTime = time
    this.history = this.history.filter((sample) => time - sample.time < 1000)
    const baseline = this.history.reduce((sum, sample) => sum + sample.energy, 0) / Math.max(1, this.history.length)
    const onset = this.history.length >= 10 && energy > Math.max(0.003, baseline * 1.7) &&
      energy - this.previous > Math.max(0.001, baseline * 0.3) && time - this.lastOnset >= 240
    this.history.push({ time, energy })
    this.previous = energy
    if (time - this.lastOnset > 4000) { this.onsets = []; this.confirmations = 0 }
    if (!onset) return
    this.lastOnset = time
    this.onsets = [...this.onsets.filter((onset) => time - onset < 16000), time].slice(-24)
    const bpm = estimateTempo(this.onsets, reference)
    if (bpm === undefined) { this.confirmations = 0; return }
    this.confirmations = Math.abs(bpm - this.candidate) <= 3 ? this.confirmations + 1 : 1
    this.candidate = bpm
    return this.confirmations >= 3 ? bpm : undefined
  }
}

export function bandEnergy(spectrum: Float32Array, sampleRate: number, fftSize: number, focus: TempoFocus): number {
  const [low, high] = focus === 'kick' ? [40, 180] : focus === 'snare' ? [700, 5000] : [40, 5000]
  const from = Math.max(1, Math.ceil(low * fftSize / sampleRate))
  const to = Math.min(spectrum.length - 1, Math.floor(high * fftSize / sampleRate))
  let power = 0
  for (let bin = from; bin <= to; bin++) power += 10 ** (spectrum[bin] / 10)
  return Math.sqrt(power / Math.max(1, to - from + 1))
}
