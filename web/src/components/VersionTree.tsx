import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ActionName, Episode, Stage, VersionNode } from '../types'
import { Button, Modal, StatusBadge, TextArea } from './ui'

export const STAGES: Stage[] = ['story', 'storyboard', 'media', 'final']

export const STAGE_LABEL: Record<Stage, string> = {
  story: '生成故事',
  storyboard: '拆分分镜',
  media: '生产画面',
  final: '合成成片',
}

// 每个阶段自身的动作名（「重做」即用它在父节点下重跑本阶段）。
const STAGE_ACTION: Record<Stage, ActionName> = {
  story: 'story',
  storyboard: 'storyboard',
  media: 'produce',
  final: 'compose',
}

// 下一阶段（「继续」从当前节点往下派生一步）。
const NEXT_STAGE: Record<Stage, Stage | ''> = {
  story: 'storyboard',
  storyboard: 'media',
  media: 'final',
  final: '',
}

// 「重做」的按钮文案：必须写清它会额外产出一版（v+1），而不是覆盖当前这版。
const REROLL_LABEL: Record<Stage, string> = {
  story: '再生成一版故事',
  storyboard: '重新拆一版分镜',
  media: '重新生产一版画面',
  final: '重新合成一版成片',
}

// 「重做」弹框里对本阶段提示词的说明；空串表示本阶段不调用模型、不给输入框。
const NOTE_HINT: Record<Stage, string> = {
  story: '会作为「本版附加要求」交给故事模型，例如「改成从行刑前一夜倒叙，不写少年经历」。',
  storyboard: '会作为「本版附加要求」交给分镜模型，例如「把朝堂争论压到三个镜头以内，多给空镜」。',
  media: '会追加到每个镜头的画面描述，例如「夜景压暗，油灯是唯一光源」。',
  final: '',
}

/** 版本树动作的附加参数：from 指定作用节点，reroll 开新版本，note 是本版附加要求。 */
export type TreeActionExtra = { from?: string; reroll?: boolean; note?: string }

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
  onAction: (a: ActionName, extra?: TreeActionExtra) => void
  onActivate: (id: string) => void
  onDelete: (id: string) => void
}) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const cardRefs = useRef(new Map<string, HTMLDivElement>())
  const [edges, setEdges] = useState<{ d: string; active: boolean }[]>([])
  // 正在「重做」的节点：非空时弹框收集本版附加要求。
  const [rerolling, setRerolling] = useState<VersionNode | null>(null)

  // upstreamLabel 描述某节点的派生来源，供弹框讲清「这一版是从哪来的」。
  const upstreamLabel = useCallback(
    (n: VersionNode) => {
      if (!n.parent_id) return '同一个起点'
      const p = ep.nodes.find((x) => x.id === n.parent_id)
      return p ? `「${STAGE_LABEL[p.stage]} v${p.attempt + 1}」` : '上游那一版'
    },
    [ep.nodes],
  )

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

  // 已经派生过的「父节点|下一阶段」组合。这些节点上的「继续」是多余的：
  // 「继续」不带 reroll，只会复用右侧那个已存在的版本、把指针挪过去，
  // 效果等同于在那个子节点上点「设为当前使用」——所以直接不显示。
  const derived = useMemo(() => {
    const s = new Set<string>()
    for (const n of ep.nodes) if (n.parent_id) s.add(`${n.parent_id}|${n.stage}`)
    return s
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
                list.map((n) => {
                  const nx = NEXT_STAGE[n.stage]
                  return (
                    <NodeCard
                      key={n.id}
                      node={n}
                      nextExists={nx !== '' && derived.has(`${n.id}|${nx}`)}
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
                      onReroll={() => setRerolling(n)}
                    />
                  )
                })
              )}
            </div>
          )
        })}
      </div>
      {rerolling && (
        <RerollModal
          key={rerolling.id}
          node={rerolling}
          upstream={upstreamLabel(rerolling)}
          pending={pending}
          onClose={() => setRerolling(null)}
          onConfirm={(note) => {
            const n = rerolling
            setRerolling(null)
            onAction(STAGE_ACTION[n.stage], { from: n.parent_id ?? '', reroll: true, note })
          }}
        />
      )}
    </div>
  )
}

function NodeCard({
  node,
  nextExists,
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
  onReroll,
}: {
  node: VersionNode
  nextExists: boolean
  active: boolean
  onActivePath: boolean
  selected: boolean
  busy: boolean
  pending: boolean
  registerRef: (id: string, el: HTMLDivElement | null) => void
  onSelect: () => void
  onAction: (a: ActionName, extra?: TreeActionExtra) => void
  onActivate: () => void
  onDelete: () => void
  onReroll: () => void
}) {
  const disabled = busy || pending
  const next = NEXT_STAGE[node.stage]
  const done = node.status === 'done'
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
          {active && <span className="ml-auto text-[10px] rounded bg-gold-500/20 px-1.5 py-0.5 text-gold-500">当前使用</span>}
        </div>
        <div className="mt-1.5 text-xs text-paper-300/60 truncate" title={nodeSummary(node)}>
          {nodeSummary(node)}
        </div>
        {node.note && (
          <div className="mt-1 text-[11px] text-gold-500/70 line-clamp-2" title={node.note}>
            本版要求：{node.note}
          </div>
        )}
        <div className="mt-0.5 text-[10px] text-paper-300/25 truncate" title={node.id}>
          {node.id}
        </div>
        {node.error && <div className="mt-1 text-[11px] text-seal-500 line-clamp-2">{node.error}</div>}
      </button>
      {selected && (
        <div className="border-t border-ink-800 px-2.5 py-2 space-y-1.5">
          <div className="text-[10px] text-paper-300/40">
            对「{STAGE_LABEL[node.stage]} v{node.attempt + 1}」操作
          </div>
          <div className="flex flex-wrap gap-1.5">
            {node.status === 'running' ? (
              <span className="text-[11px] text-gold-500/80">正在执行本步…</span>
            ) : done ? (
              next && (
                <>
                  {/* 下一步已经存在时不显示「继续」：它只会复用右边那个版本。 */}
                  {!nextExists && (
                    <Button
                      variant="primary"
                      className="px-2 py-1 text-[11px]"
                      disabled={disabled}
                      onClick={() => onAction(STAGE_ACTION[next], { from: node.id })}
                    >
                      ▶ 继续：{STAGE_LABEL[next]}
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    className="px-2 py-1 text-[11px]"
                    disabled={disabled}
                    title={`从「${STAGE_LABEL[node.stage]} v${node.attempt + 1}」沿这条线补齐到成片，已完成的镜头自动复用`}
                    onClick={() => onAction('run', { from: node.id })}
                  >
                    ⚡ 一键跑到底
                  </Button>
                </>
              )
            ) : (
              // 失败/未产出的节点不能「继续」——下游拿不到它的内容，只会报错，
              // 所以只给「重跑本步」：同派生输入、同节点、不新增版本。
              <Button
                variant="primary"
                className="px-2 py-1 text-[11px]"
                disabled={disabled}
                onClick={() => onAction(STAGE_ACTION[node.stage], { from: node.parent_id ?? '' })}
              >
                ▶ 重跑本步（重试，不新增版本）
              </Button>
            )}
            <Button variant="outline" className="px-2 py-1 text-[11px]" disabled={disabled} onClick={onReroll}>
              ↻ 重做：{REROLL_LABEL[node.stage]}
            </Button>
            {!active && (
              <Button variant="outline" className="px-2 py-1 text-[11px]" disabled={disabled} onClick={onActivate}>
                设为当前使用
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
              删除本节点及下游
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

/**
 * 重做弹框：换一版必须问清方向，否则同一个派生输入反复抽、用户无从控制。
 * 填了内容会进派生键（得到新版本），留空等价于「同条件再抽一次」。
 * 合成阶段是本地 ffmpeg 拼接、不调用模型，因此只做确认、不给输入框。
 */
function RerollModal({
  node,
  upstream,
  pending,
  onClose,
  onConfirm,
}: {
  node: VersionNode
  upstream: string
  pending: boolean
  onClose: () => void
  onConfirm: (note: string) => void
}) {
  const [note, setNote] = useState('')
  const hint = NOTE_HINT[node.stage]
  const steered = hint !== ''
  return (
    <Modal open onClose={onClose} title={REROLL_LABEL[node.stage]}>
      <div className="space-y-4">
        <p className="text-sm leading-6 text-paper-300/70">
          {node.parent_id ? (
            <>
              以 <span className="text-paper-100">{upstream}</span> 为输入另开一个新版本；
            </>
          ) : (
            <>这是整条流水线的起点，会与当前这一版并列；</>
          )}
          当前的「{STAGE_LABEL[node.stage]} v{node.attempt + 1}」完整保留，不会被覆盖。
        </p>
        {steered ? (
          <>
            <div className="space-y-2">
              <div className="text-xs text-paper-300/45">这一版要往哪个方向改？（可留空）</div>
              <TextArea
                rows={3}
                value={note}
                autoFocus
                onChange={(e) => setNote(e.target.value)}
                placeholder="例如：把结尾停在开门那一刻，不要交代后续"
              />
              <p className="text-xs text-paper-300/40">{hint}</p>
            </div>
            <p className="text-xs text-paper-300/35">
              留空＝同条件再抽一次（模型重新生成，产出通常不同）；填了内容会记在版本上，卡片和详情里都会显示「本版要求」。
            </p>
          </>
        ) : (
          <p className="text-xs text-paper-300/45">
            合成是本地 ffmpeg 拼接，不调用模型，没有可填的提示词。画面与旁白沿用上游，不会重新生成。
          </p>
        )}
        <div className="flex justify-end gap-2.5 pt-1">
          <Button variant="ghost" onClick={onClose}>
            取消
          </Button>
          <Button variant="primary" disabled={pending} onClick={() => onConfirm(note.trim())}>
            {steered && note.trim() === '' ? '同条件重做一版' : '按这个方向重做一版'}
          </Button>
        </div>
      </div>
    </Modal>
  )
}
