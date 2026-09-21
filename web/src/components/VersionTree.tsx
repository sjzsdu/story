import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ActionName, Episode, Stage, VersionNode } from '../types'
import { Button, StatusBadge } from './ui'

export const STAGES: Stage[] = ['story', 'storyboard', 'media', 'final']

export const STAGE_LABEL: Record<Stage, string> = {
  story: '生成故事',
  storyboard: '拆分分镜',
  media: '生产画面',
  final: '合成成片',
}

// 每个阶段自身的动作名（「换一版」即用它在父节点下重跑本阶段）。
const STAGE_ACTION: Record<Stage, ActionName> = {
  story: 'story',
  storyboard: 'storyboard',
  media: 'produce',
  final: 'compose',
}

// 下一阶段（「从此处继续」从当前节点往下派生一步）。
const NEXT_STAGE: Record<Stage, Stage | ''> = {
  story: 'storyboard',
  storyboard: 'media',
  media: 'final',
  final: '',
}

/** activePath 从活跃节点回溯到根，返回正序路径（与 Go 侧 Episode.ActivePath 一致）。 */
export function activePath(ep: Episode): VersionNode[] {
  const idMap = new Map(ep.nodes.map((n) => [n.id, n]))
  const rev: VersionNode[] = []
  for (let id = ep.active_node_id || '', step = 0; id && step <= ep.nodes.length; step++) {
    const n = idMap.get(id)
    if (!n) break
    rev.push(n)
    id = n.parent_id ?? ''
  }
  return rev.reverse()
}

function nodeSummary(n: VersionNode): string {
  switch (n.stage) {
    case 'story':
      return n.story ? `《${n.story.title}》` : '尚未产出'
    case 'storyboard':
      return n.storyboard ? `${n.storyboard.scenes.length} 镜` : '尚未产出'
    case 'media': {
      const clip = (n.clips ?? []).filter((c) => c.path).length
      const audio = (n.audios ?? []).filter((a) => a.path).length
      return `画面 ${clip} · 旁白 ${audio}`
    }
    case 'final':
      return `${(n.outputs ?? []).length} 个成片`
    default:
      return ''
  }
}

type Rect = { left: number; top: number; width: number; height: number }

export default function VersionTree({
  ep,
  selectedId,
  busy,
  pending,
  onSelect,
  onAction,
  onActivate,
  onDelete,
}: {
  ep: Episode
  selectedId: string
  busy: boolean
  pending: boolean
  onSelect: (id: string) => void
  onAction: (a: ActionName, extra?: { from?: string; reroll?: boolean }) => void
  onActivate: (id: string) => void
  onDelete: (id: string) => void
}) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const cardRefs = useRef(new Map<string, HTMLDivElement>())
  const [edges, setEdges] = useState<{ d: string; active: boolean }[]>([])

  const activeIds = useMemo(() => new Set(activePath(ep).map((n) => n.id)), [ep])
  const byStage = useMemo(() => {
    const m = new Map<Stage, VersionNode[]>()
    for (const s of STAGES) m.set(s, [])
    for (const n of ep.nodes) {
      const list = m.get(n.stage)
      if (list) list.push(n)
    }
    for (const list of m.values()) list.sort((a, b) => a.attempt - b.attempt)
    return m
  }, [ep.nodes])

  // 用 DOM 实测位置画父→子连线：不引入图库，纯 SVG 手绘。
  const measure = useCallback(() => {
    const box = containerRef.current
    if (!box) return
    const base = box.getBoundingClientRect()
    const rects = new Map<string, Rect>()
    for (const [id, el] of cardRefs.current) {
      const r = el.getBoundingClientRect()
      rects.set(id, { left: r.left - base.left, top: r.top - base.top, width: r.width, height: r.height })
    }
    const paths: { d: string; active: boolean }[] = []
    for (const n of ep.nodes) {
      const p = n.parent_id ? rects.get(n.parent_id) : undefined
      const c = rects.get(n.id)
      if (!p || !c) continue
      const x1 = p.left + p.width
      const y1 = p.top + p.height / 2
      const x2 = c.left
      const y2 = c.top + c.height / 2
      const dx = Math.max(20, (x2 - x1) / 2)
      paths.push({
        d: `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`,
        active: activeIds.has(n.id) && activeIds.has(n.parent_id ?? ''),
      })
    }
    setEdges(paths)
    // selectedId 变化会展开/收起操作行，卡片高度随之改变，需要重新测量。
  }, [ep.nodes, activeIds, selectedId])

  useLayoutEffect(() => {
    measure()
    const box = containerRef.current
    if (!box) return
    const ro = new ResizeObserver(measure)
    ro.observe(box)
    return () => ro.disconnect()
  }, [measure])

  const registerRef = useCallback(
    (id: string, el: HTMLDivElement | null) => {
      if (el) cardRefs.current.set(id, el)
      else cardRefs.current.delete(id)
    },
    [],
  )

  return (
    <div className="overflow-x-auto pb-1">
      <div ref={containerRef} className="relative flex gap-8 min-w-max py-1">
        <svg className="absolute inset-0 w-full h-full pointer-events-none">
          {edges.map((e, i) => (
            <path
              key={i}
              d={e.d}
              fill="none"
              stroke={e.active ? '#c8a45c' : '#2c2c2c'}
              strokeWidth={1.5}
            />
          ))}
        </svg>
        {STAGES.map((stage) => {
          const list = byStage.get(stage) ?? []
          return (
            <div key={stage} className="relative z-10 flex flex-col gap-3 w-[236px] shrink-0">
              <div className="text-xs tracking-wider text-gold-500/80">
                {STAGE_LABEL[stage]}
                {list.length > 1 && <span className="ml-1.5 text-paper-300/35">{list.length} 版</span>}
              </div>
              {list.length === 0 ? (
                <div className="h-16 rounded-lg border border-dashed border-ink-700 grid place-items-center text-xs text-paper-300/25">
                  尚未产生版本
                </div>
              ) : (
                list.map((n) => (
                  <NodeCard
                    key={n.id}
                    node={n}
                    active={n.id === ep.active_node_id}
                    onActivePath={activeIds.has(n.id)}
                    selected={n.id === selectedId}
                    busy={busy}
                    pending={pending}
                    registerRef={registerRef}
                    onSelect={() => onSelect(n.id)}
                    onAction={onAction}
                    onActivate={() => onActivate(n.id)}
                    onDelete={() => onDelete(n.id)}
                  />
                ))
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function NodeCard({
  node,
  active,
  onActivePath,
  selected,
  busy,
  pending,
  registerRef,
  onSelect,
  onAction,
  onActivate,
  onDelete,
}: {
  node: VersionNode
  active: boolean
  onActivePath: boolean
  selected: boolean
  busy: boolean
  pending: boolean
  registerRef: (id: string, el: HTMLDivElement | null) => void
  onSelect: () => void
  onAction: (a: ActionName, extra?: { from?: string; reroll?: boolean }) => void
  onActivate: () => void
  onDelete: () => void
}) {
  const disabled = busy || pending
  const next = NEXT_STAGE[node.stage]
  const border = selected
    ? 'border-gold-500/70 bg-gold-500/10'
    : onActivePath
      ? 'border-gold-500/40 bg-ink-900/80'
      : 'border-ink-700 bg-ink-900/60'
  return (
    <div ref={(el) => registerRef(node.id, el)} className={`rounded-lg border ${border} transition-colors`}>
      <button type="button" onClick={onSelect} className="w-full text-left px-3 py-2.5">
        <div className="flex items-center gap-2">
          <span className="font-display text-sm text-paper-100">v{node.attempt + 1}</span>
          <StatusBadge status={node.status} />
          {active && <span className="ml-auto text-[10px] rounded bg-gold-500/20 px-1.5 py-0.5 text-gold-500">活跃</span>}
        </div>
        <div className="mt-1.5 text-xs text-paper-300/60 truncate" title={nodeSummary(node)}>
          {nodeSummary(node)}
        </div>
        <div className="mt-0.5 text-[10px] text-paper-300/25 truncate" title={node.id}>
          {node.id}
        </div>
        {node.error && <div className="mt-1 text-[11px] text-seal-500 line-clamp-2">{node.error}</div>}
      </button>
      {selected && (
        <div className="flex flex-wrap gap-1.5 border-t border-ink-800 px-2.5 py-2">
          {next && (
            <Button
              variant="primary"
              className="px-2 py-1 text-[11px]"
              disabled={disabled}
              onClick={() => onAction(STAGE_ACTION[next], { from: node.id })}
            >
              从此处继续
            </Button>
          )}
          <Button
            variant="outline"
            className="px-2 py-1 text-[11px]"
            disabled={disabled}
            onClick={() => onAction(STAGE_ACTION[node.stage], { from: node.parent_id ?? '', reroll: true })}
          >
            换一版
          </Button>
          {!active && (
            <Button variant="outline" className="px-2 py-1 text-[11px]" disabled={disabled} onClick={onActivate}>
              设为活跃
            </Button>
          )}
          <Button
            variant="seal"
            className="px-2 py-1 text-[11px]"
            disabled={disabled}
            onClick={() => {
              if (
                window.confirm(
                  `删除「${STAGE_LABEL[node.stage]} v${node.attempt + 1}」会级联删除它的全部下游版本节点与对应媒体文件，不可恢复。确定删除？`,
                )
              ) {
                onDelete()
              }
            }}
          >
            删除
          </Button>
        </div>
      )}
    </div>
  )
}
