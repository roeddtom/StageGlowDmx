import { useId, useState, type ReactNode } from 'react'

const storageKey = (id: string) => `stageglow.collapsed.${id}`

function readCollapsed(id: string, fallback: boolean) {
  try {
    const stored = window.localStorage.getItem(storageKey(id))
    return stored === null ? fallback : stored === '1'
  } catch { return fallback }
}

export function HelpButton({ topic, open, controls, onToggle }: { topic: string; open: boolean; controls: string; onToggle: () => void }) {
  return <button className="control-help heading-help" type="button" aria-label={`Help for ${topic}`} aria-expanded={open} aria-controls={controls} onClick={onToggle}>?</button>
}

type HeadingProps = {
  kicker?: string
  title: string
  help?: ReactNode
  action?: ReactNode
  page?: boolean
  collapsed?: boolean
  onToggle?: () => void
  summary?: ReactNode
  bodyId?: string
}

// Long explanations stay behind the ? button so the page is compact once the workflow is familiar.
export function Heading({ kicker, title, help, action, page, collapsed, onToggle, summary, bodyId }: HeadingProps) {
  const id = useId()
  const [helpOpen, setHelpOpen] = useState(false)
  const Title = page ? 'h1' : 'h2'
  return <div className={page ? 'section-heading' : 'panel-heading'}>
    <div className="heading-main">
      {kicker && <div className="section-kicker">{kicker}</div>}
      <div className="heading-title-row">
        <Title>{onToggle
          ? <button className="collapse-toggle" type="button" aria-expanded={!collapsed} aria-controls={bodyId} onClick={onToggle}><span className="collapse-chevron" aria-hidden="true" />{title}</button>
          : title}</Title>
        {help && <HelpButton topic={title} open={helpOpen} controls={`${id}-help`} onToggle={() => setHelpOpen(!helpOpen)} />}
        {collapsed && summary && <span className="collapsed-summary">{summary}</span>}
      </div>
      {help && helpOpen && <div className="control-help-text heading-help-text" id={`${id}-help`}>{help}</div>}
    </div>
    {action && <div className="heading-action">{action}</div>}
  </div>
}

export function Panel({ id, className = '', defaultCollapsed = false, children, ...heading }: Omit<HeadingProps, 'page' | 'collapsed' | 'onToggle' | 'bodyId'> & {
  id: string
  className?: string
  defaultCollapsed?: boolean
  children: ReactNode
}) {
  const bodyId = useId()
  const [collapsed, setCollapsed] = useState(() => readCollapsed(id, defaultCollapsed))
  const toggle = () => {
    const next = !collapsed
    setCollapsed(next)
    try { window.localStorage.setItem(storageKey(id), next ? '1' : '0') } catch { /* preference is optional */ }
  }
  return <section className={`panel collapsible-panel ${collapsed ? 'collapsed' : ''} ${className}`} data-panel={id}>
    <Heading {...heading} collapsed={collapsed} onToggle={toggle} bodyId={bodyId} />
    {/* Keep the body mounted so unsaved drafts survive collapsing. */}
    <div className="panel-body" id={bodyId} hidden={collapsed}>{children}</div>
  </section>
}

export function Help({ topic, children }: { topic: string; children: ReactNode }) {
  const id = useId()
  const [open, setOpen] = useState(false)
  return <div className="inline-help">
    <HelpButton topic={topic} open={open} controls={id} onToggle={() => setOpen(!open)} />
    <span className="inline-help-label">{topic}</span>
    {open && <div className="control-help-text" id={id}>{children}</div>}
  </div>
}
