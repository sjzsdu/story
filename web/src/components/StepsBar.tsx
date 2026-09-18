import type { PipelineState, StepName } from '../types'
import { StatusBadge } from './ui'

const STEP_LABEL: { name: StepName; label: string }[] = [
  { name: 'generate', label: '候选故事' },
  { name: 'pick', label: '选定' },
  { name: 'storyboard', label: '分镜' },
  { name: 'produce', label: '生产片段' },
  { name: 'compose', label: '合成成片' },
]

export default function StepsBar({ state }: { state: PipelineState }) {
  return (
    <ol className="flex flex-wrap items-center gap-y-3">
      {STEP_LABEL.map((s, i) => {
        const st = state.steps[s.name]
        const active = state.current === s.name && st.status === 'running'
        return (
          <li key={s.name} className="flex items-center">
            <div
              className={`flex items-center gap-2.5 rounded-lg border px-3.5 py-2 ${
                active
                  ? 'border-gold-500/50 bg-gold-500/10'
                  : 'border-ink-800 bg-ink-900/60'
              }`}
            >
              <span
                className={`grid place-items-center w-6 h-6 rounded-full text-xs font-display ${
                  st.status === 'done' || st.status === 'approved'
                    ? 'bg-emerald-400/20 text-emerald-300'
                    : st.status === 'failed'
                      ? 'bg-seal-600/25 text-seal-500'
                      : active
                        ? 'bg-gold-500/20 text-gold-500'
                        : 'bg-ink-800 text-paper-300/50'
                }`}
              >
                {i + 1}
              </span>
              <div className="leading-tight">
                <div className="text-sm text-paper-100">{s.label}</div>
                <div className="mt-0.5">
                  <StatusBadge status={st.status} />
                </div>
              </div>
              {st.attempts > 1 && (
                <span className="text-[11px] text-paper-300/40">×{st.attempts}</span>
              )}
            </div>
            {i < STEP_LABEL.length - 1 && <span className="mx-2 text-ink-600">→</span>}
          </li>
        )
      })}
    </ol>
  )
}
