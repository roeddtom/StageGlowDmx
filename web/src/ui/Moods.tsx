import { useEffect, useRef, useState } from 'react'
import { makeId, makeMood, makeMoodGroup, removeMood, tapBPM, validateConfig, type Config, type Mood, type MoodGroup, type RuntimeState } from '../model'
import { MoodSlider } from './MoodSlider'

export type SimulationCommand =
  | { action: 'select'; moodId: string }
  | { action: 'blackout'; enabled: boolean }
  | { action: 'master'; value: number }

type Props = {
  config: Config
  runtime: RuntimeState | null
  ready: boolean
  busy: boolean
  onUpdate: (config: Config) => void
  onCommand: (command: SimulationCommand) => Promise<void>
  onNotice: (message: string) => void
}

export function Moods({ config, runtime, ready, busy, onUpdate, onCommand, onNotice }: Props) {
  const [draft, setDraft] = useState<Mood | null>(null)
  const [error, setError] = useState('')
  const [tempoInput, setTempoInput] = useState(String(config.bpm))
  const [tempoDirty, setTempoDirty] = useState(false)
  const taps = useRef<number[]>([])
  const moods = config.moods ?? []
  const active = moods.find((mood) => mood.id === runtime?.activeMoodId)
  const hardware = runtime?.outputEnabled === true
  const loadout = config.loadouts.find((item) => item.id === config.activeLoadoutId)
  const unpatchedGroups = active?.groups.filter((group) => !loadout?.fixtures.some((fixture) => fixture.groupId === group.groupId))
  useEffect(() => {
    if (!tempoDirty) setTempoInput(String(config.bpm))
  }, [config.bpm, tempoDirty])
  const apply = () => {
    if (!draft) return
    const next = {
      ...config,
      moods: moods.some((mood) => mood.id === draft.id)
        ? moods.map((mood) => mood.id === draft.id ? draft : mood) : [...moods, draft],
    }
    const errors = validateConfig(next)
    if (errors.length) { setError(errors[0]); return }
    onUpdate(next)
    setDraft(null)
    setError('')
  }
  const editGroup = (groupId: string, patch: Partial<MoodGroup>) => {
    if (draft) setDraft({ ...draft, groups: draft.groups.map((group) => group.groupId === groupId ? { ...group, ...patch } : group) })
  }
  const tap = () => {
    const result = tapBPM(taps.current, performance.now())
    taps.current = result.taps
    if (result.bpm !== undefined) {
      setTempoInput(String(result.bpm))
      setTempoDirty(false)
      onUpdate({ ...config, bpm: result.bpm })
    }
    else onNotice('Tap again to set the global tempo.')
  }

  return <div className="moods-page">
    <div className="section-heading">
      <div><div className="section-kicker">GENTLE, EVOLVING LIGHT</div><h1>Moods</h1><p>Keep front lights steady while other groups glide through a palette. {!runtime ? 'Output state is unknown; real fixtures may still be lit.' : hardware ? 'Controls now affect real fixtures through USB.' : 'USB is disabled; mood controls affect software only.'}</p></div>
      <div className="button-row">
        <button className="button" onClick={() => { setDraft(makeMood(config.groups, true)); setError('') }}>Starter mood</button>
        <button className="button primary" onClick={() => { setDraft(makeMood(config.groups)); setError('') }}>New mood</button>
      </div>
    </div>

    <section className="panel">
      <div className="panel-heading"><div><div className="section-kicker">SAVED BEHAVIORS</div><h2>Your moods</h2><p>Selecting a mood never releases blackout. When already running, mood selection and saved edits affect output immediately.</p></div></div>
      <div className="profile-grid">
        {moods.length === 0 && <p className="empty-state">No moods yet. Create a starter mood after defining fixture groups.</p>}
        {moods.map((mood) => <article className={`profile-card ${active?.id === mood.id ? 'selected-mood' : ''}`} key={mood.id}>
          <div className="card-title-row"><h3>{mood.name}</h3>{active?.id === mood.id && <span className="profile-tag">SELECTED</span>}</div>
          <p className="subtle">Every {mood.beatsPerChange} beats | {mood.crossfadeSeconds}s crossfade | {mood.groups.length} groups</p>
          <div className="palette-swatches" aria-label={`${mood.name} palette`}>{mood.groups.flatMap((group) => group.palette.map((color, i) => <span key={`${group.groupId}-${i}`} style={{ backgroundColor: color }} title={color} />))}</div>
          <div className="button-row">
            <button className="button small secondary" disabled={!ready || busy} onClick={() => void onCommand({ action: 'select', moodId: mood.id })}>Select {mood.name}</button>
            <button className="text-button" onClick={() => { setDraft(structuredClone(mood)); setError('') }} aria-label={`Edit ${mood.name}`}>Edit</button>
            <button className="text-button" onClick={() => { setDraft({ ...structuredClone(mood), id: makeId(), name: `${mood.name} copy` }); setError('') }} aria-label={`Duplicate ${mood.name}`}>Duplicate</button>
            <button className="text-button danger-text" onClick={() => {
              if (!window.confirm(`Delete mood "${mood.name}"? This clears its Show and Pause assignments. Deleting the selected mood blacks out the generated output.`)) return
              onUpdate(removeMood(config, mood.id))
              if (draft?.id === mood.id) setDraft(null)
            }} aria-label={`Delete mood ${mood.name}`}>Delete</button>
          </div>
        </article>)}
      </div>
    </section>

    {draft && <section className="panel mood-editor">
      <div className="editor-title"><div><div className="section-kicker">MOOD EDITOR</div><h2>{draft.name || 'Untitled mood'}</h2></div><button className="text-button" onClick={() => setDraft(null)}>Cancel mood edits</button></div>
      <div className="form-grid two">
        <label className="field">Mood name<input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label>
        <label className="field">Change every (beats)<input type="number" min="0.25" max="128" step="0.25" value={draft.beatsPerChange} onChange={(event) => setDraft({ ...draft, beatsPerChange: Number(event.target.value) })} /></label>
        <MoodSlider label="Smoothness" value={draft.smoothness} min={0.05} max={1} step={0.05} endpoints={['Glide then hold', 'Continuous glide']} help="Controls how much of each change interval is spent gliding to the next colour and intensity target. At 1, the glide lasts the whole interval. Lower values reach the target sooner, then hold. This does not change the interval's tempo." onChange={(smoothness) => setDraft({ ...draft, smoothness })} />
        <MoodSlider label="Crossfade" value={draft.crossfadeSeconds} min={0} max={30} step={0.5} unit="s" help="How many seconds it takes to transition into this mood, including when restoring it from blackout. Zero switches immediately. Blackout itself is always immediate." onChange={(crossfadeSeconds) => setDraft({ ...draft, crossfadeSeconds })} />
      </div>
      <p className="subtle">Smoothness 1 glides for the whole interval; lower values glide then hold. Equal intensity bounds keep brightness fixed. UV defaults to off.</p>
      {config.groups.length === 0 && <p className="empty-state">Define groups on the Fixtures page first.</p>}
      {config.groups.map((group) => {
        const settings = draft.groups.find((item) => item.groupId === group.id)
        return <fieldset className="mood-group" key={group.id}>
          <legend>{group.name}</legend>
          <label className="check-label"><input type="checkbox" checked={Boolean(settings)} onChange={(event) => setDraft({
            ...draft, groups: event.target.checked ? [...draft.groups, makeMoodGroup(group)] : draft.groups.filter((item) => item.groupId !== group.id),
          })} /> Include {group.name}</label>
          {!settings && <p className="subtle">Unassigned fixtures stay dark in this mood.</p>}
          {settings && <>
            <div className="form-grid two">
              <label className="field">Colour mode - {group.name}<select value={settings.colorMode} onChange={(event) => editGroup(group.id, { colorMode: event.target.value as MoodGroup['colorMode'] })}><option value="static">Static (first colour)</option><option value="palette">Independent palette drift</option></select></label>
              <label className="field">UV - {group.name} (0-1)<input type="number" min="0" max="1" step="0.01" value={settings.uv} onChange={(event) => editGroup(group.id, { uv: Number(event.target.value) })} /></label>
              <MoodSlider label={`Minimum intensity - ${group.name}`} value={settings.intensityMin} min={0} max={100} step={1} scale={100} unit="%" help="The dimmest target for this group, before master brightness. Each fixture gently varies between minimum and maximum. Set both to the same percentage for steady brightness. Minimum must not exceed maximum." onChange={(intensityMin) => editGroup(group.id, { intensityMin })} />
              <MoodSlider label={`Maximum intensity - ${group.name}`} value={settings.intensityMax} min={0} max={100} step={1} scale={100} unit="%" help="The brightest target for this group, before master brightness. Keep this at or above minimum. Equal minimum and maximum values hold brightness steady; 0% is dark and 100% is full intensity." onChange={(intensityMax) => editGroup(group.id, { intensityMax })} />
            </div>
            <div className="palette-editor">
              {settings.palette.map((color, index) => <div className="palette-stop" key={index}>
                <label className="field">Colour {index + 1} - {group.name}<input type="color" value={color} onChange={(event) => editGroup(group.id, { palette: settings.palette.map((item, i) => i === index ? event.target.value : item) })} /></label>
                <button className="text-button danger-text" disabled={settings.palette.length <= 1} aria-label={`Remove colour ${index + 1} from ${group.name}`} onClick={() => editGroup(group.id, { palette: settings.palette.filter((_, i) => i !== index) })}>Remove</button>
              </div>)}
              <button className="button small" disabled={settings.palette.length >= 32} onClick={() => editGroup(group.id, { palette: [...settings.palette, '#1644ff'] })} aria-label={`Add colour to ${group.name}`}>Add colour</button>
            </div>
          </>}
        </fieldset>
      })}
      {error && <p className="inline-error" role="alert">{error}</p>}
      <div className="editor-footer"><button className="button primary" onClick={apply}>Apply mood</button><span>Changes are staged until applied and then autosaved.</span></div>
    </section>}

    <section className="panel simulation-panel">
      <div className="panel-heading"><div><div className="section-kicker">{hardware ? 'PHYSICAL USB OUTPUT / NO ART-NET' : 'SOFTWARE PREVIEW / NO ART-NET'}</div><h2>{hardware ? 'USB mood control' : 'Mood simulation'}</h2><p>The server clock keeps advancing between browser visits. Selecting a mood while running crossfades smoothly.</p></div><span className="disabled-chip">{!runtime ? 'OUTPUT UNKNOWN' : hardware ? 'USB ENABLED' : 'USB DISABLED'}</span></div>
      <p>Selected: <strong>{active?.name ?? 'None'}</strong> | {runtime ? (runtime.blackout ? 'Generated blackout latched' : 'Mood running') : 'Output state unknown; lights may still be running'}</p>
      <div className="simulation-controls">
        <button className="button danger-button" disabled={busy} onClick={() => void onCommand({ action: 'blackout', enabled: true })}>{hardware ? 'Blackout USB output' : 'Blackout simulation'}</button>
        <button className="button primary" disabled={!ready || busy || !active || !runtime?.blackout} onClick={() => {
          if (hardware && !window.confirm('Release physical USB blackout and light real fixtures with the selected mood? Check master brightness and clear the stage first.')) return
          void onCommand({ action: 'blackout', enabled: false })
        }}>{hardware ? 'Release USB blackout (real lights)' : 'Start / restore simulation'}</button>
        <label className="field">Master brightness<select value={runtime?.master ?? 1} disabled={busy || !ready} onChange={(event) => void onCommand({ action: 'master', value: Number(event.target.value) })}>{[0, 0.1, 0.25, 0.5, 0.75, 1].map((value) => <option key={value} value={value}>{Math.round(value * 100)}%</option>)}</select></label>
        <form className="tempo-form" onSubmit={(event) => {
          event.preventDefault()
          const bpm = Number(tempoInput)
          if (!Number.isFinite(bpm) || bpm < 20 || bpm > 300) { onNotice('BPM must be between 20 and 300.'); return }
          onUpdate({ ...config, bpm })
          setTempoDirty(false)
        }}>
          <label className="field">Global BPM<input type="number" min="20" max="300" step="any" value={tempoInput} onChange={(event) => { setTempoInput(event.target.value); setTempoDirty(true) }} /></label>
          <button className="button" type="submit">Apply tempo</button>
        </form>
        <button className="button" onClick={tap}>Tap tempo</button>
      </div>
      <p className="subtle">{active ? `${active.beatsPerChange} beats at ${config.bpm} BPM = ${(active.beatsPerChange * 60 / config.bpm).toFixed(2)} seconds per change.` : 'Select a saved mood, then explicitly release blackout.'} {!ready && ' Save pending or backend unavailable; previews are not current.'} {hardware && 'Values below are generated output, not fixture feedback.'}</p>
      {!config.activeLoadoutId && <p className="empty-state">Choose an active loadout on the Fixtures page to see generated fixture output.</p>}
      {unpatchedGroups && unpatchedGroups.length > 0 && <p className="subtle">No fixtures in the active loadout for: {unpatchedGroups.map((group) => config.groups.find((item) => item.id === group.groupId)?.name).join(', ')}.</p>}
      <div className="profile-grid">
        {runtime?.fixtures.map((fixture) => <article className="profile-card fixture-preview" key={fixture.id}>
          <div className="preview-light" style={{ backgroundColor: fixture.color }} aria-label={`${fixture.name} simulated colour ${fixture.color}`} />
          <div><h3>{fixture.name}</h3><p className="subtle">{config.groups.find((group) => group.id === fixture.groupId)?.name} | Intensity {(fixture.intensity * 100).toFixed(0)}% | UV {(fixture.uv * 100).toFixed(0)}%</p>
            <p className="dmx-values">DMX {fixture.startAddress}-{fixture.startAddress + fixture.channels.length - 1}: {fixture.channels.join(', ')}</p>
            {fixture.unsupportedUV && <p className="inline-error">This profile has no UV channel; UV is ignored.</p>}
          </div>
        </article>)}
      </div>
      {runtime && <details className="dmx-details"><summary>Generated universe (512 channels)</summary><pre>{runtime.dmx.map((value, i) => `${i + 1}:${value}`).join(' ')}</pre></details>}
    </section>
  </div>
}
