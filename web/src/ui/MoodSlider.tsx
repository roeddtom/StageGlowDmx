import { useId, useState } from 'react'

type Props = {
  label: string
  value: number
  min: number
  max: number
  step: number
  unit?: string
  scale?: number
  endpoints?: [string, string]
  help: string
  onChange: (value: number) => void
}

export function MoodSlider({ label, value, min, max, step, unit = '', scale = 1, endpoints, help, onChange }: Props) {
  const id = useId()
  const [helpOpen, setHelpOpen] = useState(false)
  const displayed = Number((value * scale).toFixed(6))
  const update = (next: string) => onChange(Number(next) / scale)
  return <div className="field mood-slider">
    <div className="slider-heading">
      <label htmlFor={`${id}-range`}>{label}</label>
      <button className="control-help" type="button" aria-label={`Help for ${label}`} aria-expanded={helpOpen} aria-controls={`${id}-help`} onClick={() => setHelpOpen(!helpOpen)}>?</button>
    </div>
    <div className="slider-inputs">
      <input id={`${id}-range`} type="range" min={min} max={max} step={step} value={displayed} aria-valuetext={`${displayed}${unit}`} aria-describedby={helpOpen ? `${id}-help` : undefined} onChange={(event) => update(event.target.value)} />
      <div className="slider-number">
        <input type="number" aria-label={`${label} value`} aria-describedby={helpOpen ? `${id}-help` : undefined} min={min} max={max} step="any" value={displayed} onChange={(event) => update(event.target.value)} />
        {unit && <span aria-hidden="true">{unit}</span>}
      </div>
    </div>
    {endpoints && <div className="slider-endpoints"><span>{endpoints[0]}</span><span>{endpoints[1]}</span></div>}
    {helpOpen && <p className="control-help-text" id={`${id}-help`}>{help}</p>}
  </div>
}
