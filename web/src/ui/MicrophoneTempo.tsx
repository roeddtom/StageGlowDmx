import { useEffect, useRef, useState } from 'react'
import { bandEnergy, TempoDetector, type TempoFocus } from '../tempo'

type Props = {
  bpm: number
  available: boolean
  manualVersion: number
  onTempo: (bpm: number) => void
}

export function MicrophoneTempo({ bpm, available, manualVersion, onTempo }: Props) {
  const [enabled, setEnabled] = useState(false)
  const [focus, setFocus] = useState<TempoFocus>('kick')
  const [status, setStatus] = useState('Off')
  const [error, setError] = useState('')
  const latest = useRef({ bpm, available, onTempo })
  latest.current = { bpm, available, onTempo }
  useEffect(() => { setEnabled(false) }, [manualVersion])
  useEffect(() => { if (!available) setEnabled(false) }, [available])

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    let stream: MediaStream | undefined
    let context: AudioContext | undefined
    let source: MediaStreamAudioSourceNode | undefined
    let timer: number | undefined
    const release = () => {
      window.clearInterval(timer)
      source?.disconnect()
      stream?.getTracks().forEach((track) => track.stop())
      if (context && context.state !== 'closed') void context.close().catch(() => {})
    }
    const fail = (message: string) => {
      if (cancelled) return
      setError(message)
      setEnabled(false)
      release()
    }
    const visibility = () => {
      if (document.hidden) fail('Microphone stopped while Show was hidden. Tap Mic to resume.')
    }
    document.addEventListener('visibilitychange', visibility)
    const start = async () => {
      try {
        setError('')
        setStatus('Permission...')
        if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
          throw new Error('Microphone requires HTTPS (trusted certificate) or localhost. HTTP on a Pi/LAN address cannot access it.')
        }
        context = new AudioContext()
        await context.resume()
        stream = await navigator.mediaDevices.getUserMedia({
          audio: { echoCancellation: false, noiseSuppression: false, autoGainControl: false }, video: false,
        })
        if (cancelled) { release(); return }
        const analyser = context.createAnalyser()
        analyser.fftSize = 2048
        analyser.smoothingTimeConstant = 0
        source = context.createMediaStreamSource(stream)
        source.connect(analyser) // Never connect microphone audio to the speakers.
        const spectrum = new Float32Array(analyser.frequencyBinCount)
        const detector = new TempoDetector()
        let lastUpdate = -Infinity
        let lastBeat = performance.now()
        setStatus('Listening')
        stream.getAudioTracks().forEach((track) => track.addEventListener('ended', () => fail('Microphone disconnected. Tap Mic to retry.'), { once: true }))
        timer = window.setInterval(() => {
          if (cancelled || !latest.current.available) return
          const now = performance.now()
          if (context?.state !== 'running') {
            fail('Microphone audio was suspended by the browser. Keep Show visible and tap Mic to retry.')
            return
          }
          analyser.getFloatFrequencyData(spectrum)
          const detected = detector.sample(now, bandEnergy(spectrum, context.sampleRate, analyser.fftSize, focus), latest.current.bpm)
          if (detected !== undefined) {
            lastBeat = now
            setStatus(`${detected} BPM`)
            if (now - lastUpdate >= 5000 && Math.abs(detected - latest.current.bpm) >= 2) {
              const next = Math.abs(detected - latest.current.bpm) <= 10
                ? Math.round(latest.current.bpm * 0.6 + detected * 0.4) : detected
              latest.current.onTempo(Math.max(50, Math.min(210, next)))
              lastUpdate = now
            }
          } else if (now - lastBeat > 4000) {
            setStatus('Listening / holding')
          }
        }, 20)
      } catch (error) {
        fail(error instanceof Error ? error.message : 'Microphone could not start. Check browser permissions.')
      }
    }
    void start()
    return () => { cancelled = true; document.removeEventListener('visibilitychange', visibility); release() }
  }, [enabled, focus])

  return <div className="show-microphone">
    <button className={`button ${enabled ? 'mic-enabled' : ''}`} aria-pressed={enabled} disabled={!available && !enabled} onClick={() => { setError(''); setEnabled(!enabled); setStatus('Off') }} title="Local microphone tempo, 50-210 BPM. Tap tempo turns it off.">
      Mic {enabled ? 'ON' : 'OFF'}
    </button>
    <select aria-label="Microphone rhythm focus" value={focus} disabled={!available} onChange={(event) => setFocus(event.target.value as TempoFocus)}>
      <option value="kick">Kick</option><option value="snare">Snare</option><option value="full">Full band</option>
    </select>
    <span role="status">{enabled ? status : error ? 'Mic unavailable' : 'Manual tempo'}</span>
    {error && <span className="mic-error" role="alert">{error}</span>}
  </div>
}
