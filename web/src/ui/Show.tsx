import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { emptyShow, makeMood, tapBPM, type Config, type RuntimeState } from '../model'
import type { SimulationCommand } from './Moods'
import { MicrophoneTempo } from './MicrophoneTempo'

type Props = {
  config: Config
  runtime: RuntimeState | null
  ready: boolean
  busy: boolean
  onUpdate: (config: Config) => void
}

function MasterDimmer({ value, available, busy, onCommand }: {
  value: number
  available: boolean
  busy: boolean
  onCommand: (command: SimulationCommand) => Promise<void>
}) {
  const [draft, setDraft] = useState<number>()
  const inFlight = useRef(false)
  useEffect(() => {
    if (!available) { setDraft(undefined); return }
    if (busy || inFlight.current || draft === undefined) return
    const timer = window.setTimeout(() => {
      inFlight.current = true
      void onCommand({ action: 'master', value: draft }).finally(() => {
        inFlight.current = false
        setDraft((current) => current === draft ? undefined : current)
      })
    }, 80)
    return () => window.clearTimeout(timer)
  }, [draft, available, busy, onCommand])
  const level = draft ?? value
  return <label className="show-master">Master
    <input type="range" min="0" max="1" step="0.01" aria-label="Show master brightness" aria-valuetext={`${Math.round(level * 100)}%`} value={level} disabled={!available} onChange={(event) => setDraft(Number(event.target.value))} />
    <output>{Math.round(level * 100)}%</output>
  </label>
}

export function ShowSetup({ config, ready, onUpdate, onEnter }: Pick<Props, 'config' | 'ready' | 'onUpdate'> & { onEnter: () => void }) {
  const layout = config.show ?? emptyShow()
  const moods = config.moods ?? []
  const options = <><option value="">Unassigned</option>{moods.map((mood) => <option key={mood.id} value={mood.id}>{mood.type === 'base' ? '[BASE] ' : ''}{mood.name}</option>)}</>
  return <div>
    <div className="section-heading">
      <div><div className="section-kicker">PERFORMANCE LAYOUT</div><h1>Show</h1><p>Choose your mood buttons, then enter a screen-filling grid. Blackout, Pause and Tap Tempo always stay in the same bottom row.</p></div>
      <button className="button primary" disabled={!ready} onClick={onEnter}>Enter show mode</button>
    </div>
    <section className="panel">
      <div className="panel-heading"><div><h2>Show buttons</h2><p>Assignments autosave and travel with backups. Unassigned buttons stay disabled. You can repeat a mood in more than one slot.</p></div></div>
      <div className="form-grid two">
        <label className="field">Number of mood buttons<select value={layout.slots.length} onChange={(event) => {
          const count = Number(event.target.value)
          if (layout.slots.slice(count).some(Boolean) && !window.confirm('Remove assignments from the last show buttons? The moods themselves will not be deleted.')) return
          onUpdate({ ...config, show: { ...layout, slots: Array.from({ length: count }, (_, i) => layout.slots[i] ?? '') } })
        }}>{[8, 9, 10, 11, 12].map((count) => <option key={count} value={count}>{count}</option>)}</select></label>
        <label className="field">Low-light Pause mood<select value={layout.pauseMoodId} onChange={(event) => onUpdate({ ...config, show: { ...layout, pauseMoodId: event.target.value } })}><option value="">Unassigned</option>{moods.filter((mood) => mood.type !== 'base').map((mood) => <option key={mood.id} value={mood.id}>{mood.name}</option>)}</select></label>
      </div>
      <p className="subtle">Pause is an Atmosphere: it keeps the selected Base for excluded groups. Exclude Front from Pause to retain Base front lighting. Included groups override the Base. Choose gentle static lighting with UV off.</p>
      {!layout.pauseMoodId && <button className="button" disabled={!config.groups.length} onClick={() => {
        const mood = makeMood(config.groups)
        mood.name = 'Low-light pause'
        mood.groups = mood.groups.map((group) => ({ ...group, palette: ['#ffbb66'], intensityMin: 0.1, intensityMax: 0.1, uv: 0 }))
        onUpdate({ ...config, moods: [...moods, mood], show: { ...layout, pauseMoodId: mood.id } })
      }}>Create low-light Pause mood</button>}
      <div className="show-assignments form-grid two">
        {layout.slots.map((id, index) => <label className="field" key={index}>Button {index + 1}<select value={id} onChange={(event) => onUpdate({ ...config, show: { ...layout, slots: layout.slots.map((item, i) => i === index ? event.target.value : item) } })}>{options}</select></label>)}
      </div>
    </section>
    <section className="panel">
      <h2>Before performing</h2>
      <p>Enable USB deliberately on Fixtures first if you want real lights; entering Show never enables hardware or releases blackout. Simulation remains available without the interface.</p>
      <p>Base buttons change static fallback lighting; Atmosphere buttons change the atmosphere selection. Both can be selected at once in any grid position. No base clears the fallback. Mood, Base and Pause buttons immediately resume lighting from blackout, without another tap or confirmation. Blackout can also be toggled off to restore the current combination.</p>
      <p className="subtle">Show mode hides configuration and navigation. Leaving requires confirmation and does not stop output. Browser fullscreen is optional. A disconnected browser does not stop the server; output may continue.</p>
    </section>
  </div>
}

export function Show({ config, runtime, ready, busy, onUpdate, onCommand, onExit, message, onDismiss }: Props & {
  onCommand: (command: SimulationCommand) => Promise<void>
  onExit: () => void
  message: string
  onDismiss: () => void
}) {
  const layout = config.show ?? emptyShow()
  const taps = useRef<number[]>([])
  const [tapHint, setTapHint] = useState(false)
  const [fullscreenError, setFullscreenError] = useState('')
  const [manualVersion, setManualVersion] = useState(0)
  const active = config.moods.find((mood) => mood.id === runtime?.activeMoodId)
  const base = config.moods.find((mood) => mood.id === runtime?.activeBaseMoodId)
  const pause = config.moods.find((mood) => mood.id === layout.pauseMoodId)
  const hardware = runtime?.outputEnabled === true
  const fault = runtime?.output.state === 'fault'
  const canPlay = ready && !busy && !fault
  const tap = () => {
    if (!runtime) return
    setManualVersion((version) => version + 1)
    const result = tapBPM(taps.current, performance.now())
    taps.current = result.taps
    setTapHint(result.bpm === undefined)
    if (result.bpm !== undefined) onUpdate({ ...config, bpm: result.bpm })
  }
  const fullscreen = async () => {
    try {
      if (document.fullscreenElement) await document.exitFullscreen()
      else await document.documentElement.requestFullscreen()
      setFullscreenError('')
    } catch { setFullscreenError('Browser fullscreen unavailable. The show grid still fills this window.') }
  }
  return <main className={`show-screen ${runtime?.blackout ? 'is-blacked-out' : ''}`} aria-label="Show performance controls">
    <header className="show-toolbar">
      <div className={`show-status ${!runtime || fault ? 'show-warning' : ''}`} role="status">
        <strong>{!runtime ? 'DISCONNECTED - OUTPUT UNKNOWN' : fault ? 'USB FAULT - LIGHTS MAY STAY ON' : hardware ? 'USB OUTPUT' : 'SIMULATION - USB DISABLED'}</strong>
        <span title={`Base: ${base?.name ?? 'None'} | Atmosphere: ${active?.name ?? 'None'}`}>{runtime ? `${runtime.blackout ? 'BLACKOUT' : 'Playing'}: ${active?.name ?? 'None'} | Base: ${base?.name ?? 'None'}` : 'Server may still be running lights. No commands queued.'}</span>
      </div>
      <div className="show-tools">
        <button className="button" onClick={() => void fullscreen()}>Fullscreen</button>
        <button className="button" onClick={() => {
          if (!window.confirm('Leave show mode and unlock editing? Lighting will continue unchanged.')) return
          if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
          onExit()
        }}>Exit show</button>
      </div>
      <div className="show-levels">
        <button className="button show-no-base" disabled={!canPlay || !base} onClick={() => void onCommand({ action: 'clear-base' })}>No base</button>
        <MasterDimmer value={runtime?.master ?? 1} available={ready && !!runtime && !fault} busy={busy} onCommand={onCommand} />
      </div>
      <MicrophoneTempo bpm={config.bpm} available={ready && !!runtime && !fault} manualVersion={manualVersion} onTempo={(bpm) => onUpdate({ ...config, bpm })} />
    </header>
    {(message || fullscreenError) && <div className="show-message" role="alert"><span>{message || fullscreenError}</span><button aria-label="Dismiss show message" onClick={() => { onDismiss(); setFullscreenError('') }}>x</button></div>}
    <div className="show-grid" aria-label="Mood buttons" style={{ '--show-rows': Math.ceil(layout.slots.length / 4), '--show-portrait-rows': Math.ceil(layout.slots.length / 2) } as CSSProperties}>
      {layout.slots.map((id, index) => {
        const mood = config.moods.find((item) => item.id === id)
        const isBase = mood?.type === 'base'
        const selected = Boolean(mood && (isBase ? runtime?.activeBaseMoodId : runtime?.activeMoodId) === mood.id)
        return <button key={index} className={`show-mood ${isBase ? 'show-base' : ''} ${selected && !runtime?.blackout ? 'is-selected' : ''}`} disabled={!canPlay || !mood} aria-pressed={selected && !runtime?.blackout} onClick={() => { if (mood) void onCommand({ action: 'play', moodId: mood.id }) }}>
          <span className="show-slot-number">{index + 1}{isBase ? ' / BASE' : ''}</span>
          <strong>{mood?.name ?? 'Unassigned'}</strong>
          <span className="show-slot-state">{selected ? (runtime?.blackout ? 'Tap to resume' : 'PLAYING') : mood ? isBase ? 'Static fallback' : `${mood.beatsPerChange} beats` : 'Assign in Show setup'}</span>
          {mood && <span className="show-palette" aria-hidden="true">{mood.groups.flatMap((group) => group.palette).slice(0, 8).map((color, i) => <i key={i} style={{ backgroundColor: color }} />)}</span>}
        </button>
      })}
    </div>
    <div className="show-fixed" aria-label="Fixed performance buttons">
      <button className={`show-action show-blackout ${runtime?.blackout ? 'is-selected' : ''}`} disabled={busy || Boolean(runtime?.blackout && (!canPlay || (!active && !base)))} aria-pressed={runtime?.blackout ?? false} onClick={() => void onCommand({ action: 'blackout', enabled: !runtime || !runtime.blackout })}><strong>Blackout</strong><span>{!runtime ? 'State unknown' : runtime.blackout ? active || base ? 'ON / tap to restore' : 'ON / select a mood' : 'OFF / tap to black out'}</span></button>
      <button className={`show-action show-pause ${pause && runtime?.activeMoodId === pause.id && !runtime?.blackout ? 'is-selected' : ''}`} disabled={!canPlay || !pause} aria-pressed={Boolean(pause && runtime?.activeMoodId === pause.id && !runtime?.blackout)} onClick={() => { if (pause) void onCommand({ action: 'play', moodId: pause.id }) }}><strong>Pause</strong><span>{pause ? runtime?.activeMoodId === pause.id ? runtime.blackout ? 'Tap to resume' : 'PLAYING' : 'Low light' : 'Not assigned'}</span></button>
      <button className="show-action show-tap" disabled={!runtime || fault} onClick={tap}><strong>Tap tempo</strong><span>{tapHint ? 'Tap again' : `${config.bpm} BPM`}</span></button>
    </div>
  </main>
}
