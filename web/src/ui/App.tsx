import { useCallback, useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react'
import {
  channelKinds, makeId, makeTemplate, templates, validateConfig,
  type ChannelKind, type Config, type Fixture, type Group, type Loadout, type Profile, type ProfileChannel, type RuntimeState,
} from '../model'
import { Moods, type SimulationCommand } from './Moods'
import { Output } from './Output'
import { Show, ShowSetup } from './Show'

type Page = 'fixtures' | 'moods' | 'show'

class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message)
  }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(url, {
      ...init,
      headers: { ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
    })
  } catch {
    throw new ApiError('Backend connection lost. Local changes are preserved.', 0)
  }
  if (!response.ok) {
    let message = `Request failed (${response.status}).`
    try {
      const body = await response.json() as { error?: string }
      if (body.error) message = body.error
    } catch { /* The server may not return JSON for transport errors. */ }
    throw new ApiError(message, response.status)
  }
  return response.json() as Promise<T>
}

const json = (body: unknown): RequestInit => ({ method: 'PUT', body: JSON.stringify(body) })
const input = (label: string, value: string | number, onChange: (value: string) => void, props: Record<string, unknown> = {}) => (
  <label className="field">{label}<input value={value} onChange={(event) => onChange(event.target.value)} {...props} /></label>
)

export function App() {
  const [config, setConfig] = useState<Config | null>(null)
  const [runtime, setRuntime] = useState<RuntimeState | null>(null)
  const [backendConnected, setBackendConnected] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [reload, setReload] = useState(0)
  const [page, setPage] = useState<Page>('fixtures')
  const [performing, setPerforming] = useState(false)
  const [notice, setNotice] = useState('')
  const [saveError, setSaveError] = useState('')
  const [remoteConflict, setRemoteConflict] = useState(false)
  const [editorEpoch, setEditorEpoch] = useState(0)
  const [saveState, setSaveState] = useState<'saved' | 'pending' | 'saving' | 'error'>('saved')
  const [editVersion, setEditVersion] = useState(0)
  const [commandBusy, setCommandBusy] = useState(false)
  const commandVersion = useRef(0)
  const configRef = useRef<Config | null>(null)
  const versionRef = useRef(0)
  const savedVersionRef = useRef(0)
  const serverBaselineRef = useRef<Config | null>(null)
  const savingRef = useRef(false)
  const timerRef = useRef<number | undefined>()

  const acceptConfig = useCallback((next: Config) => {
    configRef.current = next
    setConfig(next)
  }, [])

  useEffect(() => {
    let cancelled = false
    setLoadError('')
    setConfig(null)
    Promise.all([
      request<Config>('/api/config'),
      request<RuntimeState>('/api/state'),
      request<{ status: string }>('/api/health'),
    ]).then(([loaded, state]) => {
      if (cancelled) return
      const errors = validateConfig(loaded)
      if (errors.length) throw new Error(`The server returned an invalid configuration: ${errors[0]}`)
      configRef.current = loaded
      serverBaselineRef.current = loaded
      savedVersionRef.current = versionRef.current
      setConfig(loaded)
      setRuntime(state)
      setBackendConnected(true)
    }).catch((error: unknown) => {
      if (!cancelled) setLoadError(error instanceof Error ? error.message : 'Could not connect to StageGlow.')
    })
    return () => { cancelled = true }
  }, [reload])

  const updateConfig = useCallback((next: Config) => {
    configRef.current = next
    setConfig(next)
    const version = versionRef.current + 1
    versionRef.current = version
    setEditVersion(version)
    setSaveState('pending')
    setSaveError('')
  }, [])

  const save = useCallback(async () => {
    if (savingRef.current || !configRef.current || versionRef.current <= savedVersionRef.current) return
    savingRef.current = true
    let failed = false
    const submittedVersion = versionRef.current
    const submittedConfig = configRef.current
    let remoteChanged = false
    setSaveState('saving')
    setSaveError('')
    try {
      const latest = await request<Config>('/api/config')
      setBackendConnected(true)
      const latestErrors = validateConfig(latest)
      if (latestErrors.length) throw new Error(`The server returned an invalid configuration: ${latestErrors[0]}`)
      if (serverBaselineRef.current && JSON.stringify(latest) !== JSON.stringify(serverBaselineRef.current)) {
        remoteChanged = true
        throw new Error('The saved configuration changed on the server. Your local edits are preserved; reload to review the newer version.')
      }
      const canonical = await request<Config>('/api/config', json(submittedConfig))
      const errors = validateConfig(canonical)
      if (errors.length) throw new Error(`Server returned invalid configuration: ${errors[0]}`)
      savedVersionRef.current = submittedVersion
      serverBaselineRef.current = canonical
      if (versionRef.current === submittedVersion) acceptConfig(canonical)
      setSaveState(versionRef.current === submittedVersion ? 'saved' : 'pending')
    } catch (error) {
      failed = true
      setRemoteConflict(remoteChanged)
      if (error instanceof ApiError && error.status === 0) setBackendConnected(false)
      setSaveError(error instanceof Error ? error.message : 'Save failed.')
      setSaveState('error')
    } finally {
      savingRef.current = false
      if (versionRef.current > savedVersionRef.current && !failed) {
        window.clearTimeout(timerRef.current)
        timerRef.current = window.setTimeout(() => { void save() }, 500)
      }
    }
  }, [acceptConfig])

  useEffect(() => {
    if (editVersion <= savedVersionRef.current) return
    window.clearTimeout(timerRef.current)
    timerRef.current = window.setTimeout(() => { void save() }, 600)
    return () => window.clearTimeout(timerRef.current)
  }, [editVersion, save])

  const applyImportedConfig = (next: Config) => {
    acceptConfig(next)
    serverBaselineRef.current = next
    setEditorEpoch((epoch) => epoch + 1)
    versionRef.current += 1
    savedVersionRef.current = versionRef.current
    setEditVersion(versionRef.current)
    setSaveState('saved')
    setSaveError('')
  }

  const reloadServerConfig = async () => {
    if (!window.confirm('Discard local configuration edits and reload the newer server configuration?')) return
    try {
      const latest = await request<Config>('/api/config')
      setBackendConnected(true)
      const errors = validateConfig(latest)
      if (errors.length) throw new Error(errors[0])
      serverBaselineRef.current = latest
      acceptConfig(latest)
      versionRef.current += 1
      savedVersionRef.current = versionRef.current
      setEditVersion(versionRef.current)
      setEditorEpoch((epoch) => epoch + 1)
      setSaveState('saved')
      setSaveError('')
      setRemoteConflict(false)
      setNotice('Loaded the latest saved configuration.')
    } catch (error) {
      if (error instanceof ApiError && error.status === 0) setBackendConnected(false)
      setSaveError(error instanceof Error ? error.message : 'Could not reload the saved configuration.')
    }
  }

  const loaded = config !== null
  useEffect(() => {
    if (!loaded) return
    let cancelled = false
    let timer: number
    const poll = async () => {
      const version = commandVersion.current
      try {
        const state = await request<RuntimeState>('/api/state')
        if (!cancelled) {
          if (version === commandVersion.current) setRuntime(state)
          setBackendConnected(true)
        }
      } catch {
        if (!cancelled) { setBackendConnected(false); setRuntime(null) }
      }
      if (!cancelled) timer = window.setTimeout(() => { void poll() }, page === 'moods' || performing ? 150 : 1000)
    }
    void poll()
    return () => { cancelled = true; window.clearTimeout(timer) }
  }, [loaded, page, performing])

  const simulation = async (command: SimulationCommand) => {
    if (commandBusy) return
    commandVersion.current++
    setCommandBusy(true)
    try {
      const state = await request<RuntimeState>(runtime?.outputEnabled ? '/api/lighting' : '/api/simulation', { method: 'POST', body: JSON.stringify(command) })
      setRuntime(state)
      setBackendConnected(true)
    } catch (error) {
      if (error instanceof ApiError && error.status === 0) { setBackendConnected(false); setRuntime(null) }
      setNotice(error instanceof Error ? error.message : 'Lighting command failed.')
    } finally {
      commandVersion.current++
      setCommandBusy(false)
    }
  }

  const enableOutput = async (enabled: boolean) => {
    if (commandBusy) return
    commandVersion.current++
    setCommandBusy(true)
    try {
      setRuntime(await request<RuntimeState>('/api/output', { method: 'POST', body: JSON.stringify({ enabled }) }))
      setBackendConnected(true)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Output command failed.')
      try { setRuntime(await request<RuntimeState>('/api/state')) }
      catch { setRuntime(null); setBackendConnected(false) }
    } finally {
      commandVersion.current++
      setCommandBusy(false)
    }
  }

  if (!config) {
    return (
      <main className="connect-screen">
        <Brand />
        <section className="connect-card">
          <div className="eyebrow">LOCAL CONTROL PANEL</div>
          <h1>{loadError ? 'Cannot reach the controller' : 'Connecting to StageGlow'}</h1>
          <p>{loadError || 'Loading your saved setup. Configuration remains untouched until the server responds.'}</p>
          {loadError && <button className="button primary" onClick={() => setReload((count) => count + 1)}>Retry connection</button>}
        </section>
      </main>
    )
  }

  if (performing) return <Show config={config} runtime={runtime} ready={saveState !== 'error' && backendConnected && runtime !== null} busy={commandBusy} onUpdate={updateConfig} onCommand={simulation} onExit={() => setPerforming(false)} message={saveError || notice} onDismiss={() => { setSaveError(''); setNotice('') }} />

  return (
    <div className="app-shell">
      <header className="topbar">
        <Brand />
        <div className="topbar-right">
          <div className={`save-indicator ${saveState}`} aria-live="polite">
            <span className="status-dot" />
            {saveState === 'saved' ? 'Saved' : saveState === 'pending' ? 'Changes pending' : saveState === 'saving' ? 'Saving' : 'Save failed'}
          </div>
          <button className="button quiet export-button" onClick={() => { window.location.href = '/api/config/export' }}>Export backup</button>
        </div>
      </header>

      <div className="safety-banner" role="status">
        <span className="safety-icon">!</span>
        <span><strong>{!runtime ? 'Output state unknown.' : runtime.outputEnabled ? 'Physical USB output enabled.' : runtime.output.state === 'fault' ? 'USB output fault - check fixtures.' : 'USB output disabled / simulation only.'}</strong> {!runtime ? 'Connection lost; lights may still be running.' : runtime.output.state === 'fault' ? 'The interface may retain its last frame. Art-Net unavailable.' : 'Art-Net unavailable. Frame writes do not confirm fixture state.'}</span>
        <span className={runtime?.blackout ? 'blackout-pill' : 'blackout-pill unconfirmed'}><i /> {runtime ? `${runtime.outputEnabled ? 'USB' : 'SIM'} ${runtime.blackout ? 'BLACKOUT' : 'RUNNING'}` : 'UNKNOWN'}</span>
        <button className="button danger-button small" disabled={commandBusy} onClick={() => void simulation({ action: 'blackout', enabled: true })}>Blackout</button>
        {(runtime?.outputEnabled || !runtime) && <button className="button small" disabled={commandBusy} onClick={() => void enableOutput(false)}>Disable USB</button>}
      </div>

      <nav className="main-nav" aria-label="Main navigation">
        {(['fixtures', 'moods', 'show'] as Page[]).map((item) => (
          <button key={item} className={page === item ? 'nav-item active' : 'nav-item'} onClick={() => setPage(item)}>
            <span className={`nav-mark ${item}`} aria-hidden="true" />
            {item === 'fixtures' ? 'Fixtures' : item === 'moods' ? 'Moods' : 'Show'}
          </button>
        ))}
        <div className="nav-spacer" />
        <div className={backendConnected ? 'connection-state' : 'connection-state disconnected'}><span className={backendConnected ? 'status-dot online' : 'status-dot'} /> {backendConnected ? 'Backend connected' : 'Backend disconnected'}</div>
      </nav>

      <main className="content">
        <div hidden={page !== 'fixtures'}>
          <Output config={config} runtime={runtime} ready={saveState === 'saved' && backendConnected && runtime !== null} busy={commandBusy} onUpdate={updateConfig} onEnable={enableOutput} onNotice={setNotice} />
          {runtime?.outputEnabled && <p className="inline-error">Patch editing is locked while USB output is enabled. Black out and disable USB above first.</p>}
          <fieldset className="patch-controls" disabled={!runtime || runtime.outputEnabled || commandBusy}><Fixtures key={editorEpoch} config={config} onUpdate={updateConfig} onNotice={setNotice} /></fieldset>
        </div>
        <div hidden={page !== 'moods'}><Moods key={editorEpoch} config={config} runtime={runtime} ready={saveState === 'saved' && backendConnected && runtime !== null} busy={commandBusy} onUpdate={updateConfig} onCommand={simulation} onNotice={setNotice} /></div>
        {page === 'show' && <ShowSetup config={config} ready={saveState === 'saved' && backendConnected && runtime !== null} onUpdate={updateConfig} onEnter={() => { setNotice(''); setPerforming(true) }} />}
        <Backup saveState={saveState} onImported={applyImportedConfig} onNotice={setNotice} onConnection={setBackendConnected} />
      </main>

      <footer className="footer">
        <span>StageGlow local setup</span>
        <span>{runtime ? (runtime.outputEnabled ? 'ENTTEC USB output' : 'Mood simulation') : 'Output state unknown'} | Art-Net disabled</span>
      </footer>
      {(saveError || notice) && (
        <div className={saveError ? 'toast error' : 'toast'} role="alert">
          <span>{saveError || notice}</span>
          {saveError && <button className="text-button" onClick={() => {
            if (remoteConflict) void reloadServerConfig()
            else { setSaveError(''); setSaveState('pending'); void save() }
          }}>{remoteConflict ? 'Reload server version' : 'Retry save'}</button>}
          <button className="toast-close" aria-label="Dismiss message" onClick={() => { setSaveError(''); setNotice('') }}>x</button>
        </div>
      )}
    </div>
  )
}

function Brand() {
  return <a className="brand" href="#" aria-label="StageGlow home">
    <span className="brand-symbol"><i /><i /><i /></span>
    <span>STAGE<span className="brand-light">GLOW</span><small>LIGHTING SETUP</small></span>
  </a>
}

function SectionHeading({ kicker, title, description, action }: { kicker: string; title: string; description: string; action?: React.ReactNode }) {
  return <div className="section-heading">
    <div><div className="section-kicker">{kicker}</div><h1>{title}</h1><p>{description}</p></div>
    {action}
  </div>
}

function Fixtures({ config, onUpdate, onNotice }: { config: Config; onUpdate: (next: Config) => void; onNotice: (message: string) => void }) {
  const [selectedLoadout, setSelectedLoadout] = useState(config.activeLoadoutId)
  const active = config.loadouts.find((loadout) => loadout.id === selectedLoadout)
  const currentLoadout = active ?? config.loadouts.find((loadout) => loadout.id === config.activeLoadoutId) ?? config.loadouts[0]
  const [groupName, setGroupName] = useState('')
  const [groupDrafts, setGroupDrafts] = useState<Record<string, string>>({})
  const [newLoadoutName, setNewLoadoutName] = useState('')
  const [fixtureDraft, setFixtureDraft] = useState<Fixture | null>(null)
  const [profileDraft, setProfileDraft] = useState<Profile | null>(null)
  const [profileMessage, setProfileMessage] = useState('')
  const [settings, setSettings] = useState({ destinationIp: config.artNet.destinationIp, portAddress: String(config.artNet.portAddress), bpm: String(config.bpm) })
  const [settingsDirty, setSettingsDirty] = useState(false)
  useEffect(() => {
    if (!settingsDirty) setSettings({ destinationIp: config.artNet.destinationIp, portAddress: String(config.artNet.portAddress), bpm: String(config.bpm) })
  }, [config, settingsDirty])

  const update = (patch: Partial<Config>) => onUpdate({ ...config, ...patch })
  const changeLoadout = (id: string) => {
    setSelectedLoadout(id)
    update({ activeLoadoutId: id })
  }
  const addGroup = (event: FormEvent) => {
    event.preventDefault()
    if (!groupName.trim()) return
    update({ groups: [...config.groups, { id: makeId(), name: groupName.trim() }] })
    setGroupName('')
  }
  const applyGroupName = (group: Group) => {
    const name = (groupDrafts[group.id] ?? group.name).trim()
    if (!name) { onNotice('Group name cannot be blank.'); return }
    update({ groups: config.groups.map((item) => item.id === group.id ? { ...item, name } : item) })
    setGroupDrafts({ ...groupDrafts, [group.id]: name })
  }
  const deleteGroup = (group: Group) => {
    if (config.moods.some((mood) => mood.groups.some((settings) => settings.groupId === group.id))) {
      onNotice(`"${group.name}" is used by moods. Remove it from those moods before deleting the group.`)
      return
    }
    if (config.loadouts.some((loadout) => loadout.fixtures.some((fixture) => fixture.groupId === group.id))) {
      onNotice(`"${group.name}" is assigned to fixtures. Reassign them before deleting this group.`)
      return
    }
    if (window.confirm(`Delete group "${group.name}"?`)) update({ groups: config.groups.filter((item) => item.id !== group.id) })
  }
  const addLoadout = (event: FormEvent) => {
    event.preventDefault()
    const name = newLoadoutName.trim()
    if (!name) return
    const loadout = { id: makeId(), name, fixtures: [] }
    update({ loadouts: [...config.loadouts, loadout], activeLoadoutId: loadout.id })
    setSelectedLoadout(loadout.id)
    setNewLoadoutName('')
  }
  const duplicateLoadout = () => {
    if (!currentLoadout) return
    const copy: Loadout = {
      id: makeId(), name: `${currentLoadout.name} copy`,
      fixtures: currentLoadout.fixtures.map((fixture) => ({ ...fixture, id: makeId() })),
    }
    update({ loadouts: [...config.loadouts, copy], activeLoadoutId: copy.id })
    setSelectedLoadout(copy.id)
  }
  const deleteLoadout = () => {
    if (!currentLoadout || !window.confirm(`Delete loadout "${currentLoadout.name}"?`)) return
    const remaining = config.loadouts.filter((loadout) => loadout.id !== currentLoadout.id)
    const nextActive = remaining[0]?.id ?? ''
    update({ loadouts: remaining, activeLoadoutId: nextActive })
    setSelectedLoadout(nextActive)
    setFixtureDraft(null)
  }
  const saveFixture = (event: FormEvent) => {
    event.preventDefault()
    if (!fixtureDraft || !currentLoadout) return
    const exists = currentLoadout.fixtures.some((fixture) => fixture.id === fixtureDraft.id)
    const next = {
      ...config,
      loadouts: config.loadouts.map((loadout) => loadout.id === currentLoadout.id
        ? { ...loadout, fixtures: exists ? loadout.fixtures.map((fixture) => fixture.id === fixtureDraft.id ? fixtureDraft : fixture) : [...loadout.fixtures, fixtureDraft] }
        : loadout),
    }
    const errors = validateConfig(next)
    if (errors.length) { onNotice(errors[0]); return }
    onUpdate(next)
    setFixtureDraft(null)
  }
  const removeFixture = (fixture: Fixture) => {
    if (!currentLoadout || !window.confirm(`Remove fixture "${fixture.name}" from this loadout?`)) return
    update({ loadouts: config.loadouts.map((loadout) => loadout.id === currentLoadout.id
      ? { ...loadout, fixtures: loadout.fixtures.filter((item) => item.id !== fixture.id) } : loadout) })
    if (fixtureDraft?.id === fixture.id) setFixtureDraft(null)
  }
  const saveProfile = () => {
    if (!profileDraft) return
    const exists = config.profiles.some((profile) => profile.id === profileDraft.id)
    const profiles = exists ? config.profiles.map((profile) => profile.id === profileDraft.id ? profileDraft : profile) : [...config.profiles, profileDraft]
    const errors = validateConfig({ ...config, profiles })
    if (errors.length) { setProfileMessage(errors[0]); return }
    onUpdate({ ...config, profiles })
    setProfileMessage('')
    setProfileDraft(null)
  }
  const deleteProfile = (profile: Profile) => {
    if (config.loadouts.some((loadout) => loadout.fixtures.some((fixture) => fixture.profileId === profile.id))) {
      onNotice(`"${profile.name}" is in use. Change or remove its fixtures before deleting the profile.`)
      return
    }
    if (window.confirm(`Delete profile "${profile.name}"?`)) update({ profiles: config.profiles.filter((item) => item.id !== profile.id) })
    if (profileDraft?.id === profile.id) setProfileDraft(null)
  }
  const updateChannel = (index: number, patch: Partial<ProfileChannel>) => {
    if (!profileDraft) return
    setProfileDraft({ ...profileDraft, channels: profileDraft.channels.map((channel, i) => i === index ? { ...channel, ...patch } : channel) })
  }
  const addChannel = () => {
    if (!profileDraft || profileDraft.footprint >= 512) return
    setProfileDraft({ ...profileDraft, channels: [...profileDraft.channels, { offset: profileDraft.footprint, kind: 'fixed', value: 0 }], footprint: profileDraft.footprint + 1 })
  }
  const applySettings = (event: FormEvent) => {
    event.preventDefault()
    const next = { ...config, artNet: { destinationIp: settings.destinationIp.trim(), portAddress: Number(settings.portAddress) }, bpm: Number(settings.bpm) }
    const errors = validateConfig(next)
    if (errors.length) { onNotice(errors[0]); return }
    onUpdate(next)
    setSettingsDirty(false)
  }

  return <div className="fixtures-page">
    <SectionHeading kicker="RIG CONFIGURATION" title="Fixtures" description="Define fixture channel maps, organize groups, and patch your active rig." />

    <section className="panel">
      <div className="panel-heading">
        <div><div className="section-kicker">01 / CHANNEL MAPS</div><h2>Fixture profiles</h2><p>Use your fixture manual to confirm every channel and safe value.</p></div>
        <div className="template-actions">
          <span className="tiny-label">START A TEMPLATE</span>
          <div className="button-row">{templates.map((template) => <button className="button small" key={template.name} onClick={() => {
            setProfileDraft(makeTemplate(`${template.name} fixture`, template.kinds)); setProfileMessage('')
          }}>{template.name}</button>)}</div>
        </div>
      </div>
      <div className="template-warning"><strong>Templates are suggestions, not fixture maps.</strong> Confirm channel order, shutter behavior, and safe values against the fixture manual before use.</div>
      <div className="profile-grid">
        {config.profiles.length === 0 && <div className="empty-state">No profiles saved. Choose a template to begin.</div>}
        {config.profiles.map((profile) => <article className="profile-card" key={profile.id}>
          <div className="card-title-row"><div><h3>{profile.name}</h3><span className="subtle">{profile.footprint} channels</span></div><span className="profile-tag">RGB PROFILE</span></div>
          <div className="channel-chips">{profile.channels.slice().sort((a, b) => a.offset - b.offset).map((channel) => <span className={`channel-chip ${channel.kind}`} key={channel.offset}><b>{channel.offset}</b> {channel.kind === 'fixed' ? `fixed ${channel.value}` : channel.kind}</span>)}</div>
          <div className="card-actions"><button className="text-button" onClick={() => { setProfileDraft(structuredClone(profile)); setProfileMessage('') }}>Edit map</button><button className="text-button danger-text" onClick={() => deleteProfile(profile)}>Delete</button></div>
        </article>)}
      </div>
      {profileDraft && <div className="editor-box">
        <div className="editor-title"><div><div className="section-kicker">PROFILE EDITOR</div><h3>{config.profiles.some((item) => item.id === profileDraft.id) ? 'Edit profile' : 'New template profile'}</h3></div><button className="text-button" onClick={() => setProfileDraft(null)}>Cancel</button></div>
        <div className="form-grid two">
          {input('Profile name', profileDraft.name, (name) => setProfileDraft({ ...profileDraft, name }))}
          {input('Footprint (channels)', profileDraft.footprint, (raw) => {
            const footprint = Number(raw)
            if (Number.isInteger(footprint) && footprint >= 3 && footprint <= 512) {
              const channels = [...profileDraft.channels]
              while (channels.length < footprint) channels.push({ offset: channels.length, kind: 'fixed', value: 0 })
              setProfileDraft({ ...profileDraft, footprint, channels: channels.slice(0, footprint).map((channel, i) => ({ ...channel, offset: channel.offset < footprint ? channel.offset : i })) })
            } else setProfileDraft({ ...profileDraft, footprint })
          }, { type: 'number', min: 3, max: 512 })}
        </div>
        <div className="channel-editor">
          <div className="channel-editor-head"><span>OFFSET</span><span>FUNCTION</span><span>SAFE VALUE</span><span>BLACKOUT VALUE</span></div>
          {profileDraft.channels.map((channel, index) => <div className="channel-editor-row" key={index}>
            <label className="sr-only" htmlFor={`offset-${index}`}>Channel offset {index}</label>
            <input id={`offset-${index}`} aria-label={`Offset for channel row ${index + 1}`} type="number" min="0" max={profileDraft.footprint - 1} value={channel.offset} onChange={(event) => updateChannel(index, { offset: Number(event.target.value) })} />
            <label className="sr-only" htmlFor={`kind-${index}`}>Channel function {index}</label>
            <select id={`kind-${index}`} value={channel.kind} onChange={(event) => updateChannel(index, { kind: event.target.value as ChannelKind, value: event.target.value === 'fixed' ? channel.value ?? 0 : undefined })}>
              {channelKinds.map((kind) => <option value={kind} key={kind}>{kind}</option>)}
            </select>
            {channel.kind === 'fixed'
              ? <input aria-label={`Safe value at offset ${channel.offset}`} type="number" min="0" max="255" value={channel.value ?? 0} onChange={(event) => updateChannel(index, { value: Number(event.target.value) })} />
              : <span className="muted-cell">Mapped</span>}
            <input aria-label={`Blackout value at offset ${channel.offset} (optional)`} type="number" min="0" max="255" placeholder="None" value={channel.blackoutValue ?? ''} onChange={(event) => updateChannel(index, { blackoutValue: event.target.value === '' ? undefined : Number(event.target.value) })} />
          </div>)}
        </div>
        <button className="button small" onClick={addChannel} disabled={profileDraft.footprint >= 512}>Add fixed channel</button>
        <div className="form-grid two mix-row">
          {input('White contribution (0-1)', profileDraft.whiteMix, (raw) => setProfileDraft({ ...profileDraft, whiteMix: Number(raw) }), { type: 'number', min: 0, max: 1, step: 0.01 })}
          {input('Amber contribution (0-1)', profileDraft.amberMix, (raw) => setProfileDraft({ ...profileDraft, amberMix: Number(raw) }), { type: 'number', min: 0, max: 1, step: 0.01 })}
        </div>
        {profileMessage && <p className="inline-error">{profileMessage}</p>}
        <div className="editor-footer"><button className="button primary" onClick={saveProfile}>Apply profile</button><span>Changes are staged until applied and then autosaved.</span></div>
      </div>}
    </section>

    <div className="two-column">
      <section className="panel">
        <div className="panel-heading compact"><div><div className="section-kicker">02 / REUSABLE IDENTITIES</div><h2>Fixture groups</h2><p>Reusable groups targeted by your moods.</p></div></div>
        <div className="group-list">
          {config.groups.map((group) => <div className="group-row" key={group.id}>
            <span className="group-mark" /><input aria-label={`Group name ${group.name}`} value={groupDrafts[group.id] ?? group.name} onChange={(event) => setGroupDrafts({ ...groupDrafts, [group.id]: event.target.value })} />
            <button className="text-button" onClick={() => applyGroupName(group)}>Apply</button>
            <button className="icon-button danger-text" aria-label={`Delete ${group.name}`} onClick={() => deleteGroup(group)}>x</button>
          </div>)}
          {config.groups.length === 0 && <div className="empty-state">Create a group to assign fixtures.</div>}
        </div>
        <form className="inline-form" onSubmit={addGroup}><input aria-label="New group name" placeholder="e.g. Front" value={groupName} onChange={(event) => setGroupName(event.target.value)} /><button className="button small" type="submit">Add group</button></form>
      </section>

      <section className="panel">
        <div className="panel-heading compact"><div><div className="section-kicker">03 / RIG PATCH</div><h2>Loadout</h2><p>Patch fixture addresses from 1 to 512.</p></div></div>
        <div className="loadout-picker">
          <label className="field">Active loadout
            <select aria-label="Active loadout" value={currentLoadout?.id ?? ''} onChange={(event) => changeLoadout(event.target.value)}>
              <option value="">Select a loadout</option>{config.loadouts.map((loadout) => <option key={loadout.id} value={loadout.id}>{loadout.name}</option>)}
            </select>
          </label>
          <div className="button-row">
            <button className="button small" disabled={!currentLoadout} onClick={duplicateLoadout}>Duplicate</button>
            <button className="button small danger-button" disabled={!currentLoadout} onClick={deleteLoadout}>Delete</button>
          </div>
        </div>
        <form className="inline-form" onSubmit={addLoadout}><input aria-label="New loadout name" placeholder="New loadout name" value={newLoadoutName} onChange={(event) => setNewLoadoutName(event.target.value)} /><button className="button small" type="submit">Create</button></form>
        {currentLoadout && <>
          <div className="loadout-name"><span>{currentLoadout.name}</span><span>{currentLoadout.fixtures.length} fixtures</span></div>
          <div className="fixture-table-wrap"><table className="fixture-table"><thead><tr><th>FIXTURE</th><th>PROFILE</th><th>ADDRESS</th><th>GROUP</th><th /></tr></thead>
            <tbody>{currentLoadout.fixtures.map((fixture) => {
              const profile = config.profiles.find((item) => item.id === fixture.profileId)
              const group = config.groups.find((item) => item.id === fixture.groupId)
              return <tr key={fixture.id}><td>{fixture.name}</td><td>{profile?.name ?? 'Missing profile'}</td><td>{fixture.startAddress}{profile ? `-${fixture.startAddress + profile.footprint - 1}` : ''}</td><td>{group?.name ?? 'Missing group'}</td><td><button className="text-button" onClick={() => setFixtureDraft({ ...fixture })}>Edit</button><button className="text-button danger-text" onClick={() => removeFixture(fixture)}>Remove</button></td></tr>
            })}</tbody></table>
            {currentLoadout.fixtures.length === 0 && <div className="empty-state">This loadout has no fixtures.</div>}
          </div>
          <button className="button secondary add-fixture" onClick={() => setFixtureDraft({ id: makeId(), name: '', profileId: config.profiles[0]?.id ?? '', startAddress: 1, groupId: config.groups[0]?.id ?? '' })}>+ Add fixture</button>
          {fixtureDraft && <form className="editor-box fixture-editor" onSubmit={saveFixture}>
            <div className="editor-title"><div><div className="section-kicker">PATCH FIXTURE</div><h3>{currentLoadout.fixtures.some((item) => item.id === fixtureDraft.id) ? 'Edit fixture' : 'Add fixture'}</h3></div><button type="button" className="text-button" onClick={() => setFixtureDraft(null)}>Cancel</button></div>
            <div className="form-grid two">
              {input('Fixture name', fixtureDraft.name, (name) => setFixtureDraft({ ...fixtureDraft, name }))}
              <label className="field">Profile<select value={fixtureDraft.profileId} onChange={(event) => setFixtureDraft({ ...fixtureDraft, profileId: event.target.value })}><option value="">Choose profile</option>{config.profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.name} ({profile.footprint}ch)</option>)}</select></label>
              {input('Start address', fixtureDraft.startAddress, (raw) => setFixtureDraft({ ...fixtureDraft, startAddress: Number(raw) }), { type: 'number', min: 1, max: 512 })}
              <label className="field">Group<select value={fixtureDraft.groupId} onChange={(event) => setFixtureDraft({ ...fixtureDraft, groupId: event.target.value })}><option value="">Choose group</option>{config.groups.map((group) => <option key={group.id} value={group.id}>{group.name}</option>)}</select></label>
            </div>
            <div className="editor-footer"><button className="button primary" type="submit">Apply fixture</button><span>Address conflicts and invalid references are rejected.</span></div>
          </form>}
        </>}
        {!currentLoadout && config.loadouts.length === 0 && <div className="empty-state">Create your first loadout above.</div>}
      </section>
    </div>

    <section className="panel settings-panel">
      <div className="panel-heading compact"><div><div className="section-kicker">04 / CONNECTION & TEMPO</div><h2>Art-Net settings</h2><p>Stored for future venue output. No Art-Net packets are sent in this release.</p></div><span className="disabled-chip">ART-NET DISABLED</span></div>
      <form onSubmit={applySettings}>
        <div className="form-grid three">
          <label className="field">Node IPv4 address<input inputMode="decimal" placeholder="Blank until configured" value={settings.destinationIp} onChange={(event) => { setSettings({ ...settings, destinationIp: event.target.value }); setSettingsDirty(true) }} /></label>
          {input('Port-Address (0-32767)', settings.portAddress, (portAddress) => { setSettings({ ...settings, portAddress }); setSettingsDirty(true) }, { type: 'number', min: 0, max: 32767 })}
          {input('Global BPM (20-300)', settings.bpm, (bpm) => { setSettings({ ...settings, bpm }); setSettingsDirty(true) }, { type: 'number', min: 20, max: 300, step: 0.1 })}
        </div>
        <div className="settings-footer"><p>Use a unicast IPv4 address. Confirm Port-Address mapping with your Art-Net node.</p><button className="button primary" type="submit">Apply settings</button></div>
      </form>
    </section>
  </div>
}

function Backup({ saveState, onImported, onNotice, onConnection }: { saveState: string; onImported: (config: Config) => void; onNotice: (message: string) => void; onConnection: (connected: boolean) => void }) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const importFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    if (saveState !== 'saved') { onNotice('Wait for pending changes to save before importing a backup.'); return }
    let parsed: unknown
    try { parsed = JSON.parse(await file.text()) }
    catch { onNotice('That file is not valid JSON. Your current configuration was not changed.'); return }
    const errors = validateConfig(parsed)
    if (errors.length) { onNotice(`Backup not imported: ${errors[0]}`); return }
    if (!window.confirm('Replace the saved configuration with this backup? This cannot be undone.')) return
    setBusy(true)
    try {
      const canonical = await request<Config>('/api/config/import', { method: 'POST', body: JSON.stringify(parsed) })
      onConnection(true)
      const serverErrors = validateConfig(canonical)
      if (serverErrors.length) throw new Error(`Server returned invalid configuration: ${serverErrors[0]}`)
      onImported(canonical)
      onNotice('Backup imported and saved.')
    } catch (error) {
      if (error instanceof ApiError && error.status === 0) onConnection(false)
      onNotice(error instanceof Error ? error.message : 'Import failed.')
    }
    finally { setBusy(false) }
  }
  return <section className="panel backup-panel">
    <div className="backup-copy"><div className="section-kicker">BACKUP & RESTORE</div><h2>Configuration backup</h2><p>Export downloads the server's saved configuration. Import validates the complete file before replacing it.</p></div>
    <div className="backup-actions"><button className="button secondary" onClick={() => { window.location.href = '/api/config/export' }}>Download JSON</button>
      <button className="button" disabled={busy || saveState !== 'saved'} onClick={() => fileRef.current?.click()}>{busy ? 'Importing...' : 'Import backup'}</button>
      <input ref={fileRef} className="sr-only" type="file" accept="application/json,.json" onChange={(event) => void importFile(event)} />
    </div>
  </section>
}
