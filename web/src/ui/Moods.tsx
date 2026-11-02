import { useEffect, useRef, useState } from 'react'
import { makeEffect, makeId, makeMood, makeMoodGroup, maxHueVariance, randomBrightColor, removeMood, tapBPM, validateConfig, type Config, type Mood, type MoodGroup, type RuntimeState } from '../model'
import { MoodSlider } from './MoodSlider'
import { Heading, Help, Panel } from './Panel'
import { EffectButton } from './EffectButton'
import { EffectPreview } from './EffectPreview'

export type SimulationCommand =
  | { action: 'select'; moodId: string }
  | { action: 'play'; moodId: string }
  | { action: 'clear-base' }
  | { action: 'effect'; moodId: string }
  | { action: 'stop-effect'; moodId?: string }
  | { action: 'blackout'; enabled: boolean }
  | { action: 'master'; value: number }

type Props = {
  visible: boolean
  config: Config
  runtime: RuntimeState | null
  ready: boolean
  busy: boolean
  onUpdate: (config: Config) => void
  onCommand: (command: SimulationCommand) => Promise<void>
  onNotice: (message: string) => void
}

export function Moods({ config, runtime, ready, busy, onUpdate, onCommand, onNotice, visible }: Props) {
  const [draft, setDraft] = useState<Mood | null>(null)
  const [error, setError] = useState('')
  const [tempoInput, setTempoInput] = useState(String(config.bpm))
  const [tempoDirty, setTempoDirty] = useState(false)
  const taps = useRef<number[]>([])
  const moods = config.moods ?? []
  const active = moods.find((mood) => mood.id === runtime?.activeMoodId)
  const activeBase = moods.find((mood) => mood.id === runtime?.activeBaseMoodId)
  const baseDraft = draft?.type === 'base'
  const effectDraft = draft?.type === 'effect'
  const fixedDraft = baseDraft || effectDraft
  const activeEffect = moods.find((mood) => mood.id === runtime?.activeEffectId)
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
    if ((draft.type === 'base' || draft.type === 'effect') && next.show?.pauseMoodId === draft.id) {
      if (!window.confirm('This mood is assigned to Pause. Changing its type will clear the Pause assignment. Continue?')) return
      next.show = { ...next.show, pauseMoodId: '' }
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
    <Heading page kicker="GENTLE, EVOLVING LIGHT" title="Moods" help={<p>Keep front lights steady while other groups glide through a palette. {!runtime ? 'Output state is unknown; real fixtures may still be lit.' : hardware ? 'Controls now affect real fixtures through USB.' : 'USB is disabled; mood controls affect software only.'}</p>}
      action={<div className="button-row">
        <button className="button" onClick={() => { setDraft(makeMood(config.groups, true)); setError('') }}>Starter mood</button>
        <button className="button primary" onClick={() => { setDraft(makeMood(config.groups)); setError('') }}>New mood</button>
        <button className="button" onClick={() => { setDraft(makeEffect(config.groups)); setError('') }}>New effect</button>
      </div>} />

    <Panel id="moods-list" kicker="SAVED BEHAVIORS" title="Your moods" summary={`${moods.length} mood${moods.length === 1 ? '' : 's'}`}
      help="Choose one static Base and one Atmosphere. One temporary Effect can override selected groups, then return to the still-running moods underneath. Effects never release blackout. Held effects stop after 30 seconds if a release is lost.">
      <div className="profile-grid">
        {moods.length === 0 && <p className="empty-state">No moods yet. Create a starter mood after defining fixture groups.</p>}
        {moods.map((mood) => <article className={`profile-card ${active?.id === mood.id || activeBase?.id === mood.id || activeEffect?.id === mood.id ? 'selected-mood' : ''}`} key={mood.id}>
          <div className="card-title-row"><h3>{mood.name}</h3><span className="profile-tag">{mood.type === 'base' ? 'BASE' : mood.type === 'effect' ? 'EFFECT' : 'ATMOSPHERE'}{active?.id === mood.id || activeBase?.id === mood.id || activeEffect?.id === mood.id ? ' / SELECTED' : ''}</span></div>
          <p className="subtle">{mood.type === 'effect' ? `${mood.effect?.pattern} / ${mood.effect?.activation}` : mood.type === 'base' ? 'Static lighting' : `Every ${mood.beatsPerChange} beats`} | {mood.crossfadeSeconds}s {mood.type === 'effect' ? 'release' : 'crossfade'} | {mood.groups.length} groups</p>
          <div className="palette-swatches" aria-label={`${mood.name} palette`}>{mood.groups.flatMap((group) => group.palette.map((color, i) => <span key={`${group.groupId}-${i}`} style={{ backgroundColor: color }} title={color} />))}</div>
          <div className="button-row">
            {mood.type === 'effect'
              ? <EffectButton mood={mood} active={activeEffect?.id === mood.id} available={ready && !!runtime && !runtime.blackout && runtime.output.state !== 'fault'} busy={busy} onCommand={onCommand} className="button small secondary" />
              : <button className="button small secondary" disabled={!ready || busy} onClick={() => void onCommand({ action: 'select', moodId: mood.id })}>Select {mood.name}</button>}
            <button className="text-button" onClick={() => { setDraft(structuredClone(mood)); setError('') }} aria-label={`Edit ${mood.name}`}>Edit</button>
            {mood.type === 'effect' && <button className="text-button" onClick={() => { setDraft(structuredClone(mood)); setError('') }} aria-label={`Preview ${mood.name}`}>Preview</button>}
            <button className="text-button" onClick={() => { setDraft({ ...structuredClone(mood), id: makeId(), name: `${mood.name} copy` }); setError('') }} aria-label={`Duplicate ${mood.name}`}>Duplicate</button>
            <button className="text-button danger-text" onClick={() => {
              if (!window.confirm(`Delete mood "${mood.name}"? This clears its Show and Pause assignments. Deleting a selected Base or Atmosphere blacks out the generated output.`)) return
              onUpdate(removeMood(config, mood.id))
              if (draft?.id === mood.id) setDraft(null)
            }} aria-label={`Delete mood ${mood.name}`}>Delete</button>
          </div>
        </article>)}
      </div>
    </Panel>

    {draft && <section className="panel mood-editor">
      <div className="editor-title"><div><div className="section-kicker">MOOD EDITOR</div><h2>{draft.name || 'Untitled mood'}</h2></div><button className="text-button" onClick={() => setDraft(null)}>Cancel mood edits</button></div>
      <div className="form-grid two">
        <label className="field">Mood name<input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label>
        <label className="field">Mood type<select value={draft.type ?? 'atmosphere'} onChange={(event) => {
          const type = event.target.value as Mood['type']
          const fixed = type === 'base' || type === 'effect'
          if (fixed && draft.groups.some((group) => group.colorMode !== 'static' || group.palette.length !== 1 || group.intensityMin !== group.intensityMax) &&
            !window.confirm('Use static group targets? Each group will keep its first colour and minimum intensity as a fixed level.')) return
          const { effect, ...rest } = draft
          setDraft({ ...rest, type, ...(type === 'effect' ? { effect: effect ?? makeEffect([]).effect } : {}), hueVariance: fixed ? 0 : draft.hueVariance, groups: fixed ? draft.groups.map((group) => ({ ...group, colorMode: 'static', palette: group.palette.slice(0, 1), intensityMax: group.intensityMin })) : draft.groups })
        }}><option value="atmosphere">Atmosphere</option><option value="base">Base (static fallback)</option><option value="effect">Effect (temporary overlay)</option></select></label>
        {effectDraft && draft.effect && <>
          <label className="field">Effect pattern<select value={draft.effect.pattern} onChange={(event) => setDraft({ ...draft, effect: { ...draft.effect!, pattern: event.target.value as NonNullable<Mood['effect']>['pattern'] } })}><option value="swell">Swell</option><option value="accent">Accent</option><option value="solo">Solo highlight</option><option value="pulse">Gentle pulse</option></select></label>
          <label className="field">Activation<select value={draft.effect.activation} onChange={(event) => setDraft({ ...draft, effect: { ...draft.effect!, activation: event.target.value as NonNullable<Mood['effect']>['activation'] } })}><option value="timed">Timed one-shot</option><option value="hold">Hold (30s safety limit)</option><option value="toggle">Toggle on/off</option></select></label>
          <label className="field">{draft.effect.activation === 'timed' ? 'Duration (beats)' : 'Envelope length (beats)'}<input type="number" min="0.25" max="128" step="0.25" value={draft.effect.durationBeats} onChange={(event) => setDraft({ ...draft, effect: { ...draft.effect!, durationBeats: Number(event.target.value) } })} /></label>
          {draft.effect.pattern === 'pulse' && <label className="field">Pulse cycle (beats)<input type="number" min="0.25" max="128" step="0.25" value={draft.beatsPerChange} onChange={(event) => setDraft({ ...draft, beatsPerChange: Number(event.target.value) })} /></label>}
          <label className="check-label"><input type="checkbox" checked={draft.effect.overrideColour} onChange={(event) => setDraft({ ...draft, effect: { ...draft.effect!, overrideColour: event.target.checked } })} /> Override colour and UV (otherwise intensity only)</label>
        </>}
        {!fixedDraft && <label className="field">Change every (beats)<input type="number" min="0.25" max="128" step="0.25" value={draft.beatsPerChange} onChange={(event) => setDraft({ ...draft, beatsPerChange: Number(event.target.value) })} /></label>}
        {!fixedDraft && <MoodSlider label="Smoothness" value={draft.smoothness} min={0.05} max={1} step={0.05} endpoints={['Glide then hold', 'Continuous glide']} help="Controls how much of each change interval is spent gliding to the next colour and intensity target. At 1, the glide lasts the whole interval. Lower values reach the target sooner, then hold. This does not change the interval's tempo." onChange={(smoothness) => setDraft({ ...draft, smoothness })} />}
        {!fixedDraft && <MoodSlider label="Hue variance" value={draft.hueVariance ?? 0} min={0} max={maxHueVariance * 100} step={1} scale={100} unit="%" endpoints={['Exact colours', 'Wider shades']} help="Adds variety to groups using palette drift. Each new colour target shifts its hue randomly by up to this share of the colour wheel in either direction (15% is about 54 degrees). Saturation and brightness stay the same. Static groups are unaffected. 0% uses the palette colours exactly." onChange={(hueVariance) => setDraft({ ...draft, hueVariance: Math.min(maxHueVariance, Math.max(0, hueVariance)) })} />}
        <MoodSlider label={effectDraft ? 'Stop release' : 'Crossfade'} value={draft.crossfadeSeconds} min={0} max={30} step={0.5} unit="s" help={effectDraft ? 'Fade back to the running Base and Atmosphere when stopped manually. Timed effects have their own shaped ending; blackout cancels immediately.' : 'How many seconds it takes to transition into this mood, including when restoring it from blackout. Zero switches immediately. Blackout itself is always immediate.'} onChange={(crossfadeSeconds) => setDraft({ ...draft, crossfadeSeconds })} />
      </div>
      <Help topic={effectDraft ? 'About Effects' : baseDraft ? 'About Base lighting' : 'About this mood'}>{effectDraft ? 'Effects blend toward each included group target, then reveal the still-running Base and Atmosphere. Intensity-only preserves current colour and UV proportions; it cannot add colour to a dark RGB light. Enable colour override to light an otherwise dark group. Swell rises and falls; Accent attacks quickly and fades; Solo sustains a highlight; Pulse smoothly repeats. Held Swell rises and sustains until release. Only one effect runs at once; a new one replaces it. Timed durations follow BPM. Holds stop after 30 seconds even if the browser disconnects.' : baseDraft ? 'Base lighting is static. Include only the groups you want as fallback; an Atmosphere explicitly including a group overrides it, even at 0% intensity.' : 'Smoothness 1 glides for the whole interval. Equal intensity bounds keep brightness fixed. Exclude groups such as Front to preserve their Base lighting.'} UV defaults to off.</Help>
      {config.groups.length === 0 && <p className="empty-state">Define groups on the Fixtures page first.</p>}
      {config.groups.map((group) => {
        const settings = draft.groups.find((item) => item.groupId === group.id)
        return <fieldset className="mood-group" key={group.id}>
          <legend>{group.name}</legend>
          <label className="check-label"><input type="checkbox" checked={Boolean(settings)} onChange={(event) => setDraft({
            ...draft, groups: event.target.checked ? [...draft.groups, { ...makeMoodGroup(group), ...(fixedDraft ? { intensityMax: 0.8 } : {}) }] : draft.groups.filter((item) => item.groupId !== group.id),
          })} /> Include {group.name}</label>
          {!settings && <p className="subtle">{effectDraft ? 'This effect leaves the group unchanged.' : baseDraft ? 'No Base lighting for this group.' : 'This group uses the selected Base, or stays dark if the Base does not include it.'}</p>}
          {settings && <>
            <div className="form-grid two">
              {!fixedDraft && <label className="field">Colour mode - {group.name}<select value={settings.colorMode} onChange={(event) => editGroup(group.id, { colorMode: event.target.value as MoodGroup['colorMode'] })}><option value="static">Static (first colour)</option><option value="palette">Independent palette drift</option></select></label>}
              {(!effectDraft || draft.effect?.overrideColour) && <label className="field">UV - {group.name} (0-1)<input type="number" min="0" max="1" step="0.01" value={settings.uv} onChange={(event) => editGroup(group.id, { uv: Number(event.target.value) })} /></label>}
              <MoodSlider label={`${effectDraft ? 'Target intensity' : baseDraft ? 'Intensity' : 'Minimum intensity'} - ${group.name}`} value={settings.intensityMin} min={0} max={100} step={1} scale={100} unit="%" help={effectDraft ? 'The effect blends toward this brightness. It can brighten or dim the group, before the master fader.' : baseDraft ? 'Fixed brightness for this group before master brightness. It remains steady unless an Atmosphere overrides this group.' : 'The dimmest target for this group, before master brightness. Each fixture gently varies between minimum and maximum. Set both to the same percentage for steady brightness. Minimum must not exceed maximum.'} onChange={(intensityMin) => editGroup(group.id, fixedDraft ? { intensityMin, intensityMax: intensityMin } : { intensityMin })} />
              {!fixedDraft && <MoodSlider label={`Maximum intensity - ${group.name}`} value={settings.intensityMax} min={0} max={100} step={1} scale={100} unit="%" help="The brightest target for this group, before master brightness. Keep this at or above minimum. Equal minimum and maximum values hold brightness steady; 0% is dark and 100% is full intensity." onChange={(intensityMax) => editGroup(group.id, { intensityMax })} />}
            </div>
            {(!effectDraft || draft.effect?.overrideColour) && <div className="palette-editor">
              {settings.palette.map((color, index) => <div className="palette-stop" key={index}>
                <label className="field">Colour {index + 1} - {group.name}<input type="color" value={color} onChange={(event) => editGroup(group.id, { palette: settings.palette.map((item, i) => i === index ? event.target.value : item) })} /></label>
                <button className="text-button danger-text" disabled={settings.palette.length <= 1} aria-label={`Remove colour ${index + 1} from ${group.name}`} onClick={() => editGroup(group.id, { palette: settings.palette.filter((_, i) => i !== index) })}>Remove</button>
              </div>)}
              {!fixedDraft && <button className="button small" disabled={settings.palette.length >= 32} onClick={() => editGroup(group.id, { palette: [...settings.palette, randomBrightColor()] })} aria-label={`Add colour to ${group.name}`}>Add colour</button>}
            </div>}
          </>}
        </fieldset>
      })}
      {error && <p className="inline-error" role="alert">{error}</p>}
      {effectDraft && visible && <EffectPreview key={draft.id} config={config} mood={draft} runtime={runtime} />}
      <div className="editor-footer"><button className="button primary" onClick={apply}>Apply mood</button><span>Changes are staged until applied and then autosaved.</span></div>
    </section>}

    <Panel id="moods-simulation" className="simulation-panel" kicker={hardware ? 'PHYSICAL USB OUTPUT / NO ART-NET' : 'SOFTWARE PREVIEW / NO ART-NET'} title={hardware ? 'USB mood control' : 'Mood simulation'}
      help="The server clock keeps advancing between browser visits. Selecting a mood while running crossfades smoothly. Values shown are generated output, not fixture feedback."
      action={<span className="disabled-chip">{!runtime ? 'OUTPUT UNKNOWN' : hardware ? 'USB ENABLED' : 'USB DISABLED'}</span>}
      summary={`${active?.name ?? 'No atmosphere'} / ${activeBase?.name ?? 'No base'}${runtime?.blackout ? ' / blackout' : ''}`}>
      <p>Atmosphere: <strong>{active?.name ?? 'None'}</strong> | Base: <strong>{activeBase?.name ?? 'None'}</strong> | {runtime ? (runtime.blackout ? 'Generated blackout latched' : 'Mood running') : 'Output state unknown; lights may still be running'}</p>
      {activeEffect && <p role="status">Effect: <strong>{activeEffect.name}</strong> <button className="button small" onClick={() => void onCommand({ action: 'stop-effect' })}>Stop effect</button></p>}
      <div className="simulation-controls">
        <button className="button danger-button" disabled={busy} onClick={() => void onCommand({ action: 'blackout', enabled: true })}>{hardware ? 'Blackout USB output' : 'Blackout simulation'}</button>
        <button className="button" disabled={!ready || busy || !activeBase} onClick={() => void onCommand({ action: 'clear-base' })}>No base</button>
        <button className="button primary" disabled={!ready || busy || (!active && !activeBase) || !runtime?.blackout} onClick={() => {
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
    </Panel>
  </div>
}
