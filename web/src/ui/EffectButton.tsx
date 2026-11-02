import { useEffect, useRef, type ReactNode } from 'react'
import type { Mood } from '../model'
import type { SimulationCommand } from './Moods'

export function EffectButton({ mood, active, available, busy, onCommand, className = 'button', children }: {
  mood: Mood
  active: boolean
  available: boolean
  busy: boolean
  onCommand: (command: SimulationCommand) => Promise<void>
  className?: string
  children?: ReactNode
}) {
  const held = useRef(false)
  const starting = useRef(false)
  const command = useRef(onCommand)
  command.current = onCommand
  const hold = mood.effect?.activation === 'hold'
  const release = () => {
    if (!held.current) return
    held.current = false
    if (!starting.current) void command.current({ action: 'stop-effect', moodId: mood.id })
  }
  useEffect(() => {
    const blur = () => release()
    const hidden = () => { if (document.hidden) release() }
    window.addEventListener('blur', blur)
    document.addEventListener('visibilitychange', hidden)
    return () => {
      window.removeEventListener('blur', blur)
      document.removeEventListener('visibilitychange', hidden)
      release()
    }
  }, [mood.id])
  const start = () => {
    if (!available || busy || held.current) return
    held.current = true
    starting.current = true
    void command.current({ action: 'effect', moodId: mood.id }).finally(() => {
      starting.current = false
      // A quick tap can release before the server acknowledges the trigger.
      if (!held.current) void command.current({ action: 'stop-effect', moodId: mood.id })
    })
  }
  return <button type="button" className={className} aria-pressed={active}
    disabled={!available || (!hold && busy)} data-command-pending={busy && available}
    onPointerDown={(event) => {
      if (!hold || event.button !== 0) return
      event.preventDefault()
      event.currentTarget.setPointerCapture(event.pointerId)
      start()
    }}
    onPointerUp={release} onPointerCancel={release} onLostPointerCapture={release} onBlur={release}
    onKeyDown={(event) => {
      if (hold && (event.key === ' ' || event.key === 'Enter')) { event.preventDefault(); if (!event.repeat) start() }
    }}
    onKeyUp={(event) => { if (hold && (event.key === ' ' || event.key === 'Enter')) { event.preventDefault(); release() } }}
    onClick={() => { if (!hold && available && !busy) void onCommand({ action: 'effect', moodId: mood.id }) }}>
    {children ?? `${hold ? 'Hold' : mood.effect?.activation === 'toggle' && active ? 'Stop' : 'Trigger'} ${mood.name}`}
  </button>
}
