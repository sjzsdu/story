import { useEffect, useId, useState, type ReactNode } from 'react'
import type { NodeStatus } from '../types'

const STATUS_META: Record<NodeStatus | 'idle', { label: string; cls: string; dot: string }> = {
  pending: { label: '未开始', cls: 'text-paper-300/50 border-ink-700 bg-ink-900', dot: 'bg-paper-300/30' },
  running: { label: '进行中', cls: 'text-gold-500 border-gold-500/40 bg-gold-500/10', dot: 'bg-gold-500 animate-pulse' },
  done: { label: '已完成', cls: 'text-emerald-300 border-emerald-400/40 bg-emerald-400/10', dot: 'bg-emerald-300' },
  failed: { label: '失败', cls: 'text-seal-500 border-seal-500/40 bg-seal-600/10', dot: 'bg-seal-500' },
  idle: { label: '未开始', cls: 'text-paper-300/50 border-ink-700 bg-ink-900', dot: 'bg-paper-300/30' },
}

export function StatusBadge({ status }: { status: NodeStatus | 'idle' }) {
  const m = STATUS_META[status] ?? STATUS_META.pending
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs ${m.cls}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${m.dot}`} />
      {m.label}
    </span>
  )
}

export function Card({ title, extra, children, className = '' }: {
  title?: ReactNode
  extra?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={`rounded-xl border border-ink-800 bg-ink-900/70 ${className}`}>
      {(title || extra) && (
        <header className="flex items-center justify-between gap-3 px-5 py-3.5 border-b border-ink-800">
          <h3 className="font-display tracking-wide text-paper-100">{title}</h3>
          {extra}
        </header>
      )}
      <div className="p-5">{children}</div>
    </section>
  )
}

type BtnVariant = 'primary' | 'ghost' | 'seal' | 'outline'

const BTN_CLS: Record<BtnVariant, string> = {
  primary: 'bg-gold-500 text-ink-950 hover:bg-[#d8ad5f] disabled:bg-ink-700 disabled:text-paper-300/40',
  seal: 'bg-seal-600 text-paper-100 hover:bg-seal-500 disabled:bg-ink-700 disabled:text-paper-300/40',
  ghost: 'text-paper-300/80 hover:text-paper-100 hover:bg-ink-800 disabled:text-paper-300/30',
  outline: 'border border-ink-600 text-paper-300 hover:border-gold-500/60 hover:text-gold-500 disabled:opacity-40',
}

export function Button({
  children,
  variant = 'outline',
  className = '',
  ...rest
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: BtnVariant }) {
  return (
    <button
      className={`inline-flex items-center gap-1.5 rounded-lg px-3.5 py-2 text-sm font-medium transition-colors disabled:cursor-not-allowed ${BTN_CLS[variant]} ${className}`}
      {...rest}
    >
      {children}
    </button>
  )
}

export function Spinner({ className = '' }: { className?: string }) {
  return (
    <span className={`inline-block w-4 h-4 rounded-full border-2 border-ink-600 border-t-gold-500 animate-spin ${className}`} />
  )
}

export function Empty({ text }: { text: string }) {
  return <div className="text-center text-sm text-paper-300/40 py-10">{text}</div>
}

const FIELD_CLS =
  'w-full rounded-lg border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-paper-100 placeholder:text-paper-300/30 outline-none focus:border-gold-500/60'

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="block text-xs text-paper-300/60 mb-1.5">{label}</span>
      {children}
    </label>
  )
}

export function TextInput(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`${FIELD_CLS} ${props.className ?? ''}`} />
}

/** 多行文本输入：与 TextInput 同款式，rows 默认为 3。 */
export function TextArea(props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea rows={props.rows ?? 3} {...props} className={`${FIELD_CLS} ${props.className ?? ''}`} />
}

export function Select(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={`${FIELD_CLS} ${props.className ?? ''}`} />
}

export function ErrorBox({ children }: { children: ReactNode }) {
  if (!children) return null
  return (
    <div className="rounded-lg border border-seal-500/40 bg-seal-600/10 px-4 py-3 text-sm text-seal-500 whitespace-pre-wrap">
      {children}
    </div>
  )
}

/** 模态弹窗：点击外部区域不关闭（避免误触丢失表单内容），按 Esc 或点 ✕ 关闭。wide 用于表单字段较多的弹窗；maxWidth 可直接指定宽度档位（如 max-w-4xl）。 */
export function Modal({
  open,
  onClose,
  title,
  children,
  wide = false,
  maxWidth,
}: {
  open: boolean
  onClose: () => void
  title?: ReactNode
  children: ReactNode
  wide?: boolean
  maxWidth?: string
}) {
  const titleID = useId()
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[10vh] bg-black/60 backdrop-blur-sm"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? titleID : undefined}
        className={`w-full ${maxWidth ?? (wide ? 'max-w-2xl' : 'max-w-lg')} max-h-[80vh] overflow-y-auto rounded-xl border border-ink-700 bg-ink-900 shadow-2xl`}
      >
        <header className="flex items-center justify-between px-5 py-3.5 border-b border-ink-800">
          <h3 id={titleID} className="font-display tracking-wide text-paper-100">
            {title}
          </h3>
          <button
            type="button"
            aria-label="关闭"
            onClick={onClose}
            className="rounded px-1.5 py-0.5 text-paper-300/50 hover:text-paper-100 hover:bg-ink-800"
          >
            ✕
          </button>
        </header>
        <div className="p-5">{children}</div>
      </div>
    </div>
  )
}

/**
 * 右侧抽屉：全高侧滑工作台（对话 / 编辑器这类需要完整纵向空间的界面）。
 * 与 Modal 的区别：贴右侧、占满高度、可固定底部操作条。
 * body 始终可滚动：内容按 h-full 撑满时宽屏刚好铺满，窄屏堆叠后自动出现滚动条。
 */
export function Drawer({
  open,
  onClose,
  title,
  extra,
  footer,
  width = 'max-w-5xl',
  children,
}: {
  open: boolean
  onClose: () => void
  title?: ReactNode
  extra?: ReactNode
  footer?: ReactNode
  width?: string
  children: ReactNode
}) {
  const titleID = useId()
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? titleID : undefined}
        className={`absolute right-0 top-0 h-full w-full ${width} flex flex-col border-l border-ink-700 bg-ink-900 shadow-2xl`}
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between gap-3 px-5 py-3.5 border-b border-ink-800">
          <div className="flex items-center gap-3 min-w-0">
            <h3 id={titleID} className="font-display tracking-wide text-paper-100 shrink-0">
              {title}
            </h3>
            {extra}
          </div>
          <button
            type="button"
            aria-label="关闭"
            onClick={onClose}
            className="shrink-0 rounded px-1.5 py-0.5 text-paper-300/50 hover:text-paper-100 hover:bg-ink-800"
          >
            ✕
          </button>
        </header>
        <div className="flex-1 min-h-0 overflow-y-auto p-5">{children}</div>
        {footer && <footer className="border-t border-ink-800 px-5 py-3.5">{footer}</footer>}
      </div>
    </div>
  )
}

/** 折叠面板：点击标题行展开/收起内容。 */
export function Collapsible({
  summary,
  children,
  defaultOpen = false,
  badge,
}: {
  summary: ReactNode
  children: ReactNode
  defaultOpen?: boolean
  badge?: ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  const contentID = useId()
  return (
    <section className="rounded-xl border border-ink-800 bg-ink-900/70 overflow-hidden">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={contentID}
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center justify-between gap-3 px-5 py-3.5 hover:bg-ink-800/40 transition-colors"
      >
        <div className="flex items-center gap-2.5">
          <span className={`text-paper-300/50 text-xs transition-transform ${open ? 'rotate-90' : ''}`}>
            ▸
          </span>
          <h3 className="font-display tracking-wide text-paper-100">{summary}</h3>
        </div>
        <div className="flex items-center gap-2">{badge}</div>
      </button>
      {open && (
        <div id={contentID} className="border-t border-ink-800 p-5">
          {children}
        </div>
      )}
    </section>
  )
}
