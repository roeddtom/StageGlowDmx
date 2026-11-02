import { useEffect, useState } from 'react'
import type { Config, Mood, RuntimeState } from '../model'
import { Help } from './Panel'

type PreviewFrame = Pick<RuntimeState, 'activeEffectId' | 'fixtures' | 'dmx'>

export function EffectPreview({ config, mood, runtime }: { config: Config; mood: Mood; runtime: RuntimeState | null }) {
  const [baseID, setBaseID] = useState(runtime?.activeBaseMoodId ?? '')
  const [atmosphereID, setAtmosphereID] = useState(runtime?.activeMoodId ?? '')
  const [started, setStarted] = useState<number | null>(null)
  const [releaseAt, setReleaseAt] = useState<number | null>(null)
  const [frame, setFrame] = useState<PreviewFrame | null>(null)
  const [error, setError] = useState('')
  const settings = JSON.stringify({ mood, baseMoodId: baseID, atmosphereMoodId: atmosphereID })
  const background = JSON.stringify([config.activeLoadoutId, config.profiles, config.loadouts, config.moods, config.bpm])
  useEffect(() => {
    setStarted(null)
    setReleaseAt(null)
    setFrame(null)
    setError('')
  }, [settings, background])
  useEffect(() => {
    if (started === null) return
    const abort = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    const poll = async () => {
      try {
        const elapsed = Math.min(3600, (performance.now() - started) / 1000)
        const response = await fetch('/api/effects/preview', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: abort.signal,
          body: JSON.stringify({ ...JSON.parse(settings), elapsedSeconds: elapsed, ...(releaseAt === null ? {} : { releaseAtSeconds: releaseAt }) }),
        })
        const result = await response.json()
        if (!response.ok) throw new Error(result.error ?? 'Effect preview failed.')
        if (abort.signal.aborted) return
        setFrame(result)
        if (result.activeEffectId && elapsed < 3600) timer = setTimeout(() => { void poll() }, 100)
        else setStarted(null)
      } catch (reason) {
        if (abort.signal.aborted) return
        setError(reason instanceof Error ? reason.message : 'Effect preview failed.')
        setStarted(null)
      }
    }
    void poll()
    return () => { abort.abort(); clearTimeout(timer) }
  }, [started, settings, background, releaseAt])
  const patched = config.loadouts.find((loadout) => loadout.id === config.activeLoadoutId)?.fixtures.length
  const none = !baseID && !atmosphereID
  return <section className="effect-preview" aria-label="Effect software preview">
    <div className="editor-title"><h3>Effect preview</h3><span className="profile-tag">SOFTWARE ONLY / NO DMX OUTPUT</span></div>
    <Help topic="About effect preview">Try saved or unsaved effects without changing live lighting, mood selections, blackout or master brightness—even with USB enabled. Preview uses the active loadout, global tempo and 100% master. Background moods start from the beginning. Changing settings stops the preview; press Preview effect to run the new version.</Help>
    <div className="form-grid two">
      <label className="field">Preview Base<select value={baseID} onChange={(event) => setBaseID(event.target.value)}><option value="">None</option>{config.moods.filter((item) => item.type === 'base' && item.id !== mood.id).map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="field">Preview Atmosphere<select value={atmosphereID} onChange={(event) => setAtmosphereID(event.target.value)}><option value="">None</option>{config.moods.filter((item) => item.type !== 'base' && item.type !== 'effect' && item.id !== mood.id).map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
    </div>
    <div className="button-row">
      <button className="button primary" disabled={!patched} onClick={() => { setError(''); setFrame(null); setReleaseAt(null); setStarted(performance.now()) }}>{started === null ? 'Preview effect' : 'Restart preview'}</button>
      <button className="button" disabled={started === null || releaseAt !== null} onClick={() => {
        if (started !== null) setReleaseAt((performance.now() - started) / 1000)
      }}>Release preview effect</button>
      <button className="button" disabled={none} onClick={() => { setBaseID(''); setAtmosphereID('') }}>No background</button>
    </div>
    <p className="subtle" role="status">{started !== null ? releaseAt === null ? 'Preview running' : 'Preview releasing' : frame ? 'Preview finished' : 'Preview stopped'} | {none ? 'No background moods' : 'Selected background moods'} | Master 100%</p>
    {!patched && <p className="subtle">Patch fixtures into the active loadout on the Fixtures page to see the effect.</p>}
    {none && !mood.effect?.overrideColour && <p className="subtle">Intensity-only effects cannot add colour to dark RGB fixtures. Enable Override colour and UV to see colour from darkness. White-only spots still follow intensity.</p>}
    {error && <p className="inline-error" role="alert">{error}</p>}
    <div className="profile-grid">
      {frame?.fixtures.map((fixture) => <article className="profile-card fixture-preview" key={fixture.id}>
        <div className="preview-light" style={{ backgroundColor: fixture.color }} aria-label={`${fixture.name} preview colour ${fixture.color}`} />
        <div><h3>{fixture.name}</h3><p className="subtle">Intensity {(fixture.intensity * 100).toFixed(0)}% | UV {(fixture.uv * 100).toFixed(0)}%</p>
          <p className="dmx-values">Preview DMX {fixture.startAddress}-{fixture.startAddress + fixture.channels.length - 1}: {fixture.channels.join(', ')}</p>
          {fixture.unsupportedUV && <p className="inline-error">This profile has no UV channel; UV is ignored.</p>}
        </div>
      </article>)}
    </div>
  </section>
}
