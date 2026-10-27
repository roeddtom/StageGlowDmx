import { useRef, useState, type CSSProperties } from 'react'
import { emptyShow, makeMood, tapBPM, type Config, type RuntimeState } from '../model'
import type { SimulationCommand } from './Moods'

type Props = {
  config: Config
  runtime: RuntimeState | null
  ready: boolean
  busy: boolean
  onUpdate: (config: Config) => void
}

export function ShowSetup({ config, ready, onUpdate, onEnter }: Pick<Props, 'config' | 'ready' | 'onUpdate'> & { onEnter: () => void }) {
  const layout = config.show ?? emptyShow()
  const moods = config.moods ?? []
  const options = <><option value="">Unassigned</option>{moods.map((mood) => <option key={mood.id} value={mood.id}>{mood.name}</option>)}</>
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
        <label className="field">Low-light Pause mood<select value={layout.pauseMoodId} onChange={(event) => onUpdate({ ...config, show: { ...layout, pauseMoodId: event.target.value } })}>{options}</select></label>
      </div>
      <p className="subtle">Pause uses the assigned mood's intensity and crossfade, not an automatic dimmer. Choose gentle static lighting with UV off, or create a starting point below and adjust it in Moods.</p>
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
      <p>When running, mood and Pause buttons crossfade immediately. During blackout they select the pending mood only. Use the separate Release blackout control to restore lighting.</p>
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
  const active = config.moods.find((mood) => mood.id === runtime?.activeMoodId)
  const pause = config.moods.find((mood) => mood.id === layout.pauseMoodId)
  const hardware = runtime?.outputEnabled === true
  const fault = runtime?.output.state === 'fault'
  const canPlay = ready && !busy && !fault
  const tap = () => {
    if (!runtime) return
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
  return <main className="show-screen" aria-label="Show performance controls">
    <header className="show-toolbar">
      <div className={`show-status ${!runtime || fault ? 'show-warning' : ''}`} role="status">
        <strong>{!runtime ? 'DISCONNECTED - OUTPUT UNKNOWN' : fault ? 'USB FAULT - LIGHTS MAY STAY ON' : hardware ? 'USB OUTPUT' : 'SIMULATION - USB DISABLED'}</strong>
        <span>{runtime ? `${runtime.blackout ? 'BLACKOUT / pending' : 'Playing'}: ${active?.name ?? 'None'}` : 'Server may still be running lights. No commands queued.'}</span>
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
        <label>Master <select aria-label="Show master brightness" value={runtime?.master ?? 1} disabled={!canPlay} onChange={(event) => void onCommand({ action: 'master', value: Number(event.target.value) })}>{[0, 0.1, 0.25, 0.5, 0.75, 1].map((value) => <option key={value} value={value}>{Math.round(value * 100)}%</option>)}</select></label>
        <span>{config.bpm} BPM<small className="show-tempo-status">{!ready && runtime ? ' / save unavailable' : runtime && runtime.bpm !== config.bpm ? ' / saving tempo' : ''}</small></span>
        <button className="button show-release" disabled={!canPlay || !runtime?.blackout || !active} onClick={() => {
          if (hardware && !window.confirm('Release USB blackout and turn on real fixtures with the pending mood?')) return
          void onCommand({ action: 'blackout', enabled: false })
        }}>Release blackout</button>
      </div>
    </header>
    {(message || fullscreenError) && <div className="show-message" role="alert"><span>{message || fullscreenError}</span><button aria-label="Dismiss show message" onClick={() => { onDismiss(); setFullscreenError('') }}>x</button></div>}
    <div className="show-grid" aria-label="Mood buttons" style={{ '--show-rows': Math.ceil(layout.slots.length / 4), '--show-portrait-rows': Math.ceil(layout.slots.length / 2) } as CSSProperties}>
      {layout.slots.map((id, index) => {
        const mood = config.moods.find((item) => item.id === id)
        const selected = Boolean(mood && runtime?.activeMoodId === mood.id)
        return <button key={index} className={`show-mood ${selected ? 'is-selected' : ''}`} disabled={!canPlay || !mood} aria-pressed={selected} onClick={() => { if (mood) void onCommand({ action: 'select', moodId: mood.id }) }}>
          <span className="show-slot-number">{index + 1}</span>
          <strong>{mood?.name ?? 'Unassigned'}</strong>
          <span className="show-slot-state">{selected ? (runtime?.blackout ? 'PENDING / BLACKOUT' : 'PLAYING') : mood ? `${mood.beatsPerChange} beats` : 'Assign in Show setup'}</span>
          {mood && <span className="show-palette" aria-hidden="true">{mood.groups.flatMap((group) => group.palette).slice(0, 8).map((color, i) => <i key={i} style={{ backgroundColor: color }} />)}</span>}
        </button>
      })}
    </div>
    <div className="show-fixed" aria-label="Fixed performance buttons">
      <button className={`show-action show-blackout ${runtime?.blackout ? 'is-selected' : ''}`} disabled={busy} aria-pressed={runtime?.blackout ?? false} onClick={() => void onCommand({ action: 'blackout', enabled: true })}><strong>Blackout</strong><span>{!runtime ? 'State unknown' : runtime.blackout ? 'LATCHED' : 'Immediate'}</span></button>
      <button className={`show-action show-pause ${pause && runtime?.activeMoodId === pause.id ? 'is-selected' : ''}`} disabled={!canPlay || !pause} aria-pressed={Boolean(pause && runtime?.activeMoodId === pause.id)} onClick={() => { if (pause) void onCommand({ action: 'select', moodId: pause.id }) }}><strong>Pause</strong><span>{pause ? runtime?.activeMoodId === pause.id ? runtime.blackout ? 'PENDING / BLACKOUT' : 'PLAYING' : 'Low light' : 'Not assigned'}</span></button>
      <button className="show-action show-tap" disabled={!runtime || fault} onClick={tap}><strong>Tap tempo</strong><span>{tapHint ? 'Tap again' : `${config.bpm} BPM`}</span></button>
    </div>
  </main>
}
