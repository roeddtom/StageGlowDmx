export const channelKinds = ['red', 'green', 'blue', 'white', 'amber', 'uv', 'dimmer', 'fixed'] as const
export type ChannelKind = (typeof channelKinds)[number]

export type ProfileChannel = {
  offset: number
  kind: ChannelKind
  value?: number
  blackoutValue?: number
}

export type Profile = {
  id: string
  name: string
  footprint: number
  channels: ProfileChannel[]
  whiteMix: number
  amberMix: number
}

export type Group = { id: string; name: string }
export type Fixture = { id: string; name: string; profileId: string; startAddress: number; groupId: string }
export type Loadout = { id: string; name: string; fixtures: Fixture[] }
export type MoodGroup = {
  groupId: string
  colorMode: 'static' | 'palette'
  palette: string[]
  intensityMin: number
  intensityMax: number
  uv: number
}
export type Mood = {
  id: string
  name: string
  beatsPerChange: number
  smoothness: number
  crossfadeSeconds: number
  groups: MoodGroup[]
}
export type RuntimeState = {
  blackout: boolean
  outputEnabled: boolean
  engineStatus: string
  activeMoodId: string
  bpm: number
  master: number
  output: {
    mode: string; port: string; enabled: boolean; state: string
    error: string; framesSent: number; cleanupPending: boolean
  }
  dmx: number[]
  fixtures: {
    id: string; name: string; groupId: string; startAddress: number
    color: string; intensity: number; uv: number; unsupportedUV: boolean; channels: number[]
  }[]
}
export type Config = {
  schemaVersion: 1
  profiles: Profile[]
  groups: Group[]
  loadouts: Loadout[]
  activeLoadoutId: string
  artNet: { destinationIp: string; portAddress: number }
  bpm: number
  moods: Mood[]
  output: { mode: 'simulation' | 'enttec' | 'artnet'; serialPort: string }
}

export const emptyConfig = (): Config => ({
  schemaVersion: 1,
  profiles: [],
  groups: [],
  loadouts: [],
  activeLoadoutId: '',
  artNet: { destinationIp: '', portAddress: 0 },
  bpm: 120,
  moods: [],
  output: { mode: 'simulation', serialPort: '' },
})

export function makeMoodGroup(group: Group, evolving = false): MoodGroup {
  return {
    groupId: group.id, colorMode: evolving ? 'palette' : 'static',
    palette: evolving ? ['#1644ff', '#a020f0', '#1634a0'] : ['#ffbb66'],
    intensityMin: evolving ? 0.3 : 0.6, intensityMax: 0.6, uv: 0,
  }
}

export function makeMood(groups: Group[], starter = false): Mood {
  return {
    id: makeId(), name: starter ? 'Warm stage' : 'New mood',
    beatsPerChange: 8, smoothness: 1, crossfadeSeconds: 3,
    groups: groups.map((group) => makeMoodGroup(group, starter && !/front/i.test(group.name))),
  }
}

export function tapBPM(taps: number[], now: number): { taps: number[]; bpm?: number } {
  const last = taps.at(-1)
  if (last === undefined || now - last > 3000 || now <= last) return { taps: [now] }
  if (now - last < 200) return { taps }
  const next = [...taps, now].slice(-5)
  return { taps: next, bpm: Math.round(60000 * (next.length - 1) / (now - next[0])) }
}

export const templates: { name: string; kinds: ChannelKind[] }[] = [
  { name: 'RGB', kinds: ['red', 'green', 'blue'] },
  { name: 'RGBA', kinds: ['red', 'green', 'blue', 'amber'] },
  { name: 'RGBW', kinds: ['red', 'green', 'blue', 'white'] },
  { name: 'RGBWAUV', kinds: ['red', 'green', 'blue', 'white', 'amber', 'uv'] },
]

export function makeTemplate(name: string, kinds: ChannelKind[]): Profile {
  return {
    id: makeId(),
    name,
    footprint: kinds.length,
    channels: kinds.map((kind, offset) => ({ offset, kind })),
    whiteMix: 1,
    amberMix: 1,
  }
}

export function makeId(): string {
  return globalThis.crypto?.randomUUID?.() ?? `id-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
}

function isIntegerIn(value: unknown, min: number, max: number): value is number {
  return Number.isInteger(value) && typeof value === 'number' && value >= min && value <= max
}

function isFiniteIn(value: unknown, min: number, max: number): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= min && value <= max
}

function isText(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function isUnicastIPv4(value: string): boolean {
  if (!value) return true
  const parts = value.split('.')
  if (parts.length !== 4 || parts.some((part) => !/^(0|[1-9]\d{0,2})$/.test(part))) return false
  const octets = parts.map(Number)
  if (octets.some((part) => part > 255)) return false
  return octets[0] > 0 && octets[0] < 224
}

export function validateConfig(input: unknown): string[] {
  const errors: string[] = []
  if (!isRecord(input)) return ['Configuration must be a JSON object.']
  const checkShape = (value: unknown, label: string, required: string[], optional: string[] = []): value is Record<string, unknown> => {
    if (!isRecord(value)) {
      errors.push(`${label} must be a JSON object.`)
      return false
    }
    for (const key of required) if (!(key in value)) errors.push(`${label} is missing "${key}".`)
    for (const key of Object.keys(value)) if (![...required, ...optional].includes(key)) errors.push(`${label} contains unsupported field "${key}".`)
    return true
  }
  checkShape(input, 'Configuration', ['schemaVersion', 'profiles', 'groups', 'loadouts', 'activeLoadoutId', 'artNet', 'bpm'], ['moods', 'output'])
  const config = input as Partial<Config>
  if (config.schemaVersion !== 1) errors.push('Only configuration schema version 1 is supported.')
  if (!Array.isArray(config.profiles) || !Array.isArray(config.groups) || !Array.isArray(config.loadouts)) {
    errors.push('Profiles, groups, and loadouts must be arrays.')
    return errors
  }
  if (typeof config.activeLoadoutId !== 'string') errors.push('Active loadout ID must be text.')
  if (!checkShape(config.artNet, 'Art-Net settings', ['destinationIp', 'portAddress'])) errors.push('Art-Net settings are missing.')
  else {
    if (typeof config.artNet.destinationIp !== 'string' || !isUnicastIPv4(config.artNet.destinationIp)) {
      errors.push('Art-Net destination must be a unicast IPv4 address or blank.')
    }
    if (!isIntegerIn(config.artNet.portAddress, 0, 32767)) errors.push('Art-Net Port-Address must be between 0 and 32767.')
  }
  if (!isFiniteIn(config.bpm, 20, 300)) errors.push('BPM must be between 20 and 300.')
  if ('output' in input && checkShape(config.output, 'Output settings', ['mode', 'serialPort'])) {
    if (!['simulation', 'enttec', 'artnet'].includes(config.output.mode)) errors.push('Output mode must be simulation, enttec, or artnet.')
    const port = config.output.serialPort
    if (typeof port !== 'string' || (port !== '' && (
      !/^(COM[1-9][0-9]*|\/dev\/tty(USB|ACM)[0-9]+|\/dev\/serial\/by-id\/[A-Za-z0-9_:.+-]+)$/.test(port) ||
      port === '/dev/serial/by-id/.' || port === '/dev/serial/by-id/..'
    ))) errors.push('Serial port must be COMn, /dev/ttyUSBn, /dev/ttyACMn, or a /dev/serial/by-id/ path.')
  }

  const uniqueIds = (items: { id: string }[], label: string) => {
    const seen = new Set<string>()
    for (const item of items) {
      if (!isText(item.id)) { errors.push(`${label} IDs must not be blank.`); continue }
      if (seen.has(item.id)) errors.push(`${label} IDs must be unique.`)
      seen.add(item.id)
    }
  }
  const profiles = config.profiles.filter(isRecord) as Profile[]
  const groups = config.groups.filter(isRecord) as Group[]
  const loadouts = config.loadouts.filter(isRecord) as Loadout[]
  uniqueIds(profiles, 'Profile')
  uniqueIds(groups, 'Group')
  uniqueIds(loadouts, 'Loadout')

  for (const rawProfile of config.profiles) {
    if (!checkShape(rawProfile, 'Profile', ['id', 'name', 'footprint', 'channels', 'whiteMix', 'amberMix'])) continue
    const profile = rawProfile as Profile
    if (!isText(profile.name)) errors.push('Every profile needs a name.')
    if (!isIntegerIn(profile.footprint, 3, 512)) {
      errors.push(`Profile "${profile.name || profile.id}" footprint must be 3-512 channels.`)
      continue
    }
    if (!Array.isArray(profile.channels)) {
      errors.push(`Profile "${profile.name}" channels must be an array.`)
      continue
    }
    const offsets = new Set<number>()
    const kinds = new Set<string>()
    for (const rawChannel of profile.channels) {
      if (!checkShape(rawChannel, `Profile "${profile.name}" channel`, ['offset', 'kind'], ['value', 'blackoutValue'])) continue
      const channel = rawChannel as ProfileChannel
      if (!isIntegerIn(channel.offset, 0, profile.footprint - 1)) {
        errors.push(`Profile "${profile.name}" has a channel offset outside its footprint.`)
        continue
      }
      if (offsets.has(channel.offset)) errors.push(`Profile "${profile.name}" repeats offset ${channel.offset}.`)
      offsets.add(channel.offset)
      if (!channelKinds.includes(channel.kind)) {
        errors.push(`Profile "${profile.name}" has an unsupported channel kind.`)
        continue
      }
      if (channel.kind !== 'fixed' && kinds.has(channel.kind)) errors.push(`Profile "${profile.name}" repeats its ${channel.kind} channel.`)
      if (channel.kind !== 'fixed') kinds.add(channel.kind)
      if (channel.kind === 'fixed' && !isIntegerIn(channel.value, 0, 255)) {
        errors.push(`Profile "${profile.name}" fixed channel at offset ${channel.offset} needs a safe value from 0 to 255.`)
      }
      if (channel.value !== undefined && !isIntegerIn(channel.value, 0, 255)) errors.push(`Profile "${profile.name}" has an invalid channel value.`)
      if (channel.blackoutValue !== undefined && !isIntegerIn(channel.blackoutValue, 0, 255)) {
        errors.push(`Profile "${profile.name}" has an invalid blackout value.`)
      }
    }
    if (offsets.size !== profile.footprint) errors.push(`Profile "${profile.name}" must define every offset from 0 to ${profile.footprint - 1}.`)
    for (const kind of ['red', 'green', 'blue']) if (!kinds.has(kind)) errors.push(`Profile "${profile.name}" needs exactly one ${kind} channel.`)
    if (!isFiniteIn(profile.whiteMix, 0, 1)) errors.push(`Profile "${profile.name}" white mix must be 0-1.`)
    if (!isFiniteIn(profile.amberMix, 0, 1)) errors.push(`Profile "${profile.name}" amber mix must be 0-1.`)
  }

  for (const rawGroup of config.groups) {
    if (!checkShape(rawGroup, 'Group', ['id', 'name'])) continue
    const group = rawGroup as Group
    if (!isText(group.name)) errors.push('Every group needs a name.')
  }
  const profileIds = new Set(profiles.map((profile) => profile.id))
  const groupIds = new Set(groups.map((group) => group.id))
  for (const rawLoadout of config.loadouts) {
    if (!checkShape(rawLoadout, 'Loadout', ['id', 'name', 'fixtures'])) continue
    const loadout = rawLoadout as Loadout
    if (!isText(loadout.name) || !Array.isArray(loadout.fixtures)) {
      errors.push('Every loadout needs a name and a fixtures array.')
      continue
    }
    const fixtureIds = new Set<string>()
    const occupied = new Set<number>()
    for (const rawFixture of loadout.fixtures) {
      if (!checkShape(rawFixture, `Loadout "${loadout.name}" fixture`, ['id', 'name', 'profileId', 'startAddress', 'groupId'])) continue
      const fixture = rawFixture as Fixture
      if (!isText(fixture.name) || !isText(fixture.id)) {
        errors.push(`Loadout "${loadout.name}" has a fixture with missing identity or name.`)
        continue
      }
      if (fixtureIds.has(fixture.id)) errors.push(`Loadout "${loadout.name}" has duplicate fixture IDs.`)
      fixtureIds.add(fixture.id)
      if (!profileIds.has(fixture.profileId)) errors.push(`Fixture "${fixture.name}" references a missing profile.`)
      if (!groupIds.has(fixture.groupId)) errors.push(`Fixture "${fixture.name}" must belong to an existing group.`)
      if (!isIntegerIn(fixture.startAddress, 1, 512)) {
        errors.push(`Fixture "${fixture.name}" start address must be 1-512.`)
        continue
      }
      const profile = profiles.find((item) => item.id === fixture.profileId)
      if (!profile || !isIntegerIn(profile.footprint, 3, 512)) continue
      if (fixture.startAddress + profile.footprint - 1 > 512) {
        errors.push(`Fixture "${fixture.name}" extends past DMX address 512.`)
      }
      for (let address = fixture.startAddress; address < fixture.startAddress + profile.footprint && address <= 512; address++) {
        if (occupied.has(address)) errors.push(`Loadout "${loadout.name}" has overlapping fixtures at address ${address}.`)
        occupied.add(address)
      }
    }
  }
  const loadoutIds = new Set(loadouts.map((loadout) => loadout.id))
  if (config.activeLoadoutId && !loadoutIds.has(config.activeLoadoutId)) errors.push('Active loadout must reference an existing loadout.')
  if ('moods' in input && !Array.isArray(config.moods)) errors.push('Moods must be an array.')
  if (Array.isArray(config.moods)) {
    uniqueIds(config.moods.filter(isRecord) as Mood[], 'Mood')
    for (const rawMood of config.moods) {
      if (!checkShape(rawMood, 'Mood', ['id', 'name', 'beatsPerChange', 'smoothness', 'crossfadeSeconds', 'groups'])) continue
      const mood = rawMood as Mood
      if (!isText(mood.name)) errors.push('Every mood needs a name.')
      if (!isFiniteIn(mood.beatsPerChange, 0.25, 128)) errors.push('Beats per change must be 0.25-128.')
      if (!isFiniteIn(mood.smoothness, 0.05, 1)) errors.push('Smoothness must be 0.05-1.')
      if (!isFiniteIn(mood.crossfadeSeconds, 0, 30)) errors.push('Crossfade must be 0-30 seconds.')
      if (!Array.isArray(mood.groups)) { errors.push('Mood groups must be an array.'); continue }
      const assigned = new Set<string>()
      for (const rawGroup of mood.groups) {
        if (!checkShape(rawGroup, 'Mood group', ['groupId', 'colorMode', 'palette', 'intensityMin', 'intensityMax', 'uv'])) continue
        const group = rawGroup as MoodGroup
        if (!groupIds.has(group.groupId) || assigned.has(group.groupId)) errors.push('Mood must reference each existing group at most once.')
        assigned.add(group.groupId)
        if (!['static', 'palette'].includes(group.colorMode)) errors.push('Mood colour mode must be static or palette.')
        if (!Array.isArray(group.palette) || group.palette.length < 1 || group.palette.length > 32 ||
          group.palette.some((color) => typeof color !== 'string' || !/^#[0-9a-f]{6}$/i.test(color))) {
          errors.push('Palette must contain 1-32 #RRGGBB colours.')
        }
        if (!isFiniteIn(group.intensityMin, 0, 1) || !isFiniteIn(group.intensityMax, 0, 1) || group.intensityMin > group.intensityMax) {
          errors.push('Intensity must be an ordered range from 0 to 1.')
        }
        if (!isFiniteIn(group.uv, 0, 1)) errors.push('UV must be 0-1.')
      }
    }
  }
  return [...new Set(errors)]
}
