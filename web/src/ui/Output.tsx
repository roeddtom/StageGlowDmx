import { useEffect, useState } from 'react'
import { validateConfig, type Config, type RuntimeState } from '../model'
import { Panel } from './Panel'

type Props = {
  config: Config
  runtime: RuntimeState | null
  ready: boolean
  busy: boolean
  onUpdate: (config: Config) => void
  onEnable: (enabled: boolean) => Promise<void>
  onNotice: (message: string) => void
}

export function Output({ config, runtime, ready, busy, onUpdate, onEnable, onNotice }: Props) {
  const [draft, setDraft] = useState(config.output ?? { mode: 'simulation', serialPort: '' })
  const [dirty, setDirty] = useState(false)
  const [ports, setPorts] = useState<string[]>([])
  const [listing, setListing] = useState(false)
  const output = runtime?.output
  const locked = !runtime || Boolean(output?.enabled || output?.cleanupPending) || busy
  useEffect(() => {
    if (!dirty) setDraft(config.output ?? { mode: 'simulation', serialPort: '' })
  }, [config.output, dirty])
  const refresh = async () => {
    setListing(true)
    try {
      const response = await fetch('/api/output/ports')
      const result = await response.json() as { ports?: string[]; error?: string }
      if (!response.ok || !Array.isArray(result.ports)) throw new Error(result.error ?? 'Could not list serial ports.')
      setPorts(result.ports)
      if (!result.ports.length) onNotice('No serial ports detected. Check the USB cable and driver; you can also enter a stable device path.')
    } catch (error) {
      onNotice(error instanceof Error ? error.message : 'Could not list serial ports.')
    } finally { setListing(false) }
  }
  const modelHelp = <>
    <p>Supports ENTTEC DMX USB PRO and Eurolite USB-DMX512 PRO MK2, not Open DMX USB. Select the exact model; serial-port names do not identify it. Ports belong to the server, not this browser.</p>
    <p>Disable output before editing the patch or port. Backups exclude machine-local output settings; imports return to simulation. Entering Show automatically enables a configured USB interface in blackout. Faults require an explicit retry here.</p>
    {config.output.mode === 'eurolite-mk2' && <p>Eurolite MK2 uses 250,000 baud and has no USB PRO handshake. An enabled status confirms serial writes only, not the device model or fixture response.</p>}
    <p>Frame writes are not fixture acknowledgements. A USB fault, power loss, or server failure may leave the interface repeating its last frame. Browser loss does not stop lighting; use the fixture's physical controls if necessary.</p>
  </>
  return <Panel id="output-usb" className="output-panel" kicker="ONE UNIVERSE" title="USB output" help={modelHelp}
    action={<span className="profile-tag">{output?.enabled ? 'USB ENABLED' : output?.state.toUpperCase() ?? 'UNKNOWN'}</span>}
    summary={`${config.output.mode === 'simulation' ? 'Simulation only' : config.output.serialPort || 'No port'}`}>
    <form onSubmit={(event) => {
      event.preventDefault()
      const next = { ...config, output: draft }
      const errors = validateConfig(next)
      if (errors.length) { onNotice(errors[0]); return }
      onUpdate(next)
      setDirty(false)
    }}>
      <fieldset className="patch-controls" disabled={locked}>
        <div className="form-grid two">
          <label className="field">Output mode<select value={draft.mode} onChange={(event) => { setDraft({ ...draft, mode: event.target.value as Config['output']['mode'] }); setDirty(true) }}><option value="simulation">Simulation only</option><option value="enttec">ENTTEC DMX USB PRO</option><option value="eurolite-mk2">Eurolite USB-DMX512 PRO MK2</option><option value="artnet" disabled>Art-Net (not available)</option></select></label>
          <label className="field">Server serial port<input list="serial-ports" placeholder="COM3 or /dev/serial/by-id/..." value={draft.serialPort} onChange={(event) => { setDraft({ ...draft, serialPort: event.target.value }); setDirty(true) }} /><datalist id="serial-ports">{ports.map((port) => <option key={port} value={port} />)}</datalist></label>
        </div>
        <div className="button-row"><button className="button" type="button" disabled={listing} onClick={() => void refresh()}>{listing ? 'Scanning...' : 'Refresh serial ports'}</button><button className="button primary" type="submit">Apply output settings</button></div>
      </fieldset>
    </form>
    <div className="button-row output-actions">
      <button className="button primary" disabled={!ready || dirty || busy || !output || output.enabled || output.cleanupPending || !['enttec', 'eurolite-mk2'].includes(config.output?.mode) || !config.output.serialPort} onClick={() => {
        const model = config.output.mode === 'eurolite-mk2' ? 'Eurolite USB-DMX512 PRO MK2' : 'ENTTEC DMX USB PRO'
        if (window.confirm(`Enable physical ${model} output? Verify the interface, fixture channel map and safe fixed values first. This sends a blackout frame; releasing blackout in Moods can turn on real lights.`)) void onEnable(true)
      }}>Enable USB output (blacked out)</button>
      <button className="button danger-button" disabled={busy} onClick={() => void onEnable(false)}>Black out & disable USB</button>
    </div>
    {output && <p className="subtle">Server: <strong>{output.state}</strong> | Frames written: {output.framesSent}{output.port && ` | ${output.port}`}</p>}
    {output?.cleanupPending && <p className="inline-error" role="alert">USB cleanup pending. Do not reconnect or re-enable until cleanup finishes. A stuck driver may require a service restart.</p>}
    {output?.error && <p className="inline-error" role="alert">{output.error} Check the cable, driver, permissions, and other apps using the port; retry explicitly after cleanup.</p>}
  </Panel>
}

export function ArtNetSettings({ config, onUpdate, onNotice }: Pick<Props, 'config' | 'onUpdate' | 'onNotice'>) {
  const current = () => ({ destinationIp: config.artNet.destinationIp, portAddress: String(config.artNet.portAddress) })
  const [settings, setSettings] = useState(current)
  const [dirty, setDirty] = useState(false)
  useEffect(() => {
    if (!dirty) setSettings({ destinationIp: config.artNet.destinationIp, portAddress: String(config.artNet.portAddress) })
  }, [config.artNet, dirty])
  const edit = (patch: Partial<typeof settings>) => { setSettings({ ...settings, ...patch }); setDirty(true) }
  return <Panel id="output-artnet" className="settings-panel" kicker="FUTURE VENUE OUTPUT" title="Art-Net" defaultCollapsed
    help="Stored for future venue output. No Art-Net packets are sent in this release. Use a unicast IPv4 address and confirm the Port-Address mapping with your Art-Net node."
    action={<span className="disabled-chip">ART-NET DISABLED</span>}
    summary={config.artNet.destinationIp || 'Not configured'}>
    <form onSubmit={(event) => {
      event.preventDefault()
      const next = { ...config, artNet: { destinationIp: settings.destinationIp.trim(), portAddress: Number(settings.portAddress) } }
      const errors = validateConfig(next)
      if (errors.length) { onNotice(errors[0]); return }
      onUpdate(next)
      setDirty(false)
    }}>
      <div className="form-grid two">
        <label className="field">Node IPv4 address<input inputMode="decimal" placeholder="Blank until configured" value={settings.destinationIp} onChange={(event) => edit({ destinationIp: event.target.value })} /></label>
        <label className="field">Port-Address (0-32767)<input type="number" min="0" max="32767" value={settings.portAddress} onChange={(event) => edit({ portAddress: event.target.value })} /></label>
      </div>
      <div className="settings-footer"><span /><button className="button primary" type="submit">Apply Art-Net settings</button></div>
    </form>
  </Panel>
}