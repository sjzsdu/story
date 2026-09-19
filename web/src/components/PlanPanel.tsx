import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { CharacterSetting, EpisodeDraft } from '../types'
import { Button, Card, ErrorBox, Spinner } from './ui'

const FIELD_CLS =
  'w-full rounded-lg border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-paper-100 placeholder:text-paper-300/30 outline-none focus:border-gold-500/60'

const DEFAULT_PROMPT = '请根据这个系列的主题与体量，帮我规划一整季的分集大纲，给出建议集数与理由。'

/**
 * AI 分集策划面板：左侧多轮对话，右侧全量可编辑分集草案。
 * 会话持久化在服务端（SQLite），采纳后批量创建集，但不触发视频生产。
 */
export default function PlanPanel({ seriesId, existingTitles }: { seriesId: string; existingTitles: string[] }) {
  const queryClient = useQueryClient()
  const planKey = ['plan', seriesId]
  const seriesKey = ['series', seriesId]

  const planQuery = useQuery({
    queryKey: planKey,
    queryFn: () => api.getPlan(seriesId),
  })

  // 草案与人物设定在本地可编辑；服务端返回新版本时同步。
  const [drafts, setDrafts] = useState<EpisodeDraft[]>([])
  const [characters, setCharacters] = useState<CharacterSetting[]>([])
  useEffect(() => {
    setDrafts(planQuery.data?.drafts ?? [])
    setCharacters(planQuery.data?.characters ?? [])
  }, [planQuery.data])

  const [input, setInput] = useState('')
  const [err, setErr] = useState('')
  const [notice, setNotice] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)

  const messages = planQuery.data?.messages ?? []
  const hasSession = messages.length > 0
  const existing = new Set(existingTitles.map((t) => t.trim()))

  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [messages.length, chatLoadingSentinel(messages)])

  const chatMut = useMutation({
    mutationFn: (message: string) => api.planChat(seriesId, message),
    onSuccess: (ps) => {
      queryClient.setQueryData(planKey, ps)
      setInput('')
      setErr('')
    },
    onError: (e) => setErr((e as Error).message),
  })

  const applyMut = useMutation({
    mutationFn: () => api.planApply(seriesId, drafts, characters),
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: seriesKey })
      void queryClient.invalidateQueries({ queryKey: planKey })
      const n = res.episodes.length
      setNotice(
        n > 0
          ? `已创建 ${n} 集，人物设定已随系列保存。可在下方集列表进入生产。`
          : '草案中的集均已创建；人物设定已随系列保存。',
      )
      setErr('')
    },
    onError: (e) => setErr((e as Error).message),
  })

  const resetMut = useMutation({
    mutationFn: () => api.planReset(seriesId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: planKey })
      setDrafts([])
      setNotice('')
      setErr('')
    },
    onError: (e) => setErr((e as Error).message),
  })

  const send = (text: string) => {
    const msg = text.trim()
    if (!msg || chatMut.isPending) return
    setNotice('')
    chatMut.mutate(msg)
  }

  const updateDraft = (i: number, patch: Partial<EpisodeDraft>) => {
    setDrafts((ds) => ds.map((d, idx) => (idx === i ? { ...d, ...patch } : d)))
  }
  const removeDraft = (i: number) => setDrafts((ds) => ds.filter((_, idx) => idx !== i))

  const updateCharacter = (i: number, patch: Partial<CharacterSetting>) => {
    setCharacters((cs) => cs.map((c, idx) => (idx === i ? { ...c, ...patch } : c)))
  }
  const removeCharacter = (i: number) => setCharacters((cs) => cs.filter((_, idx) => idx !== i))
  const addCharacter = () =>
    setCharacters((cs) => [...cs, { name: '', identity: '', appearance: '', temperament: '' }])

  const pendingCount = drafts.filter((d) => !existing.has(d.title.trim()) && d.title.trim()).length
  const canApply = pendingCount > 0 || characters.length > 0

  return (
    <Card
      title="AI 分集策划"
      extra={
        <span className="text-xs text-paper-300/45">对话持久保存 · 采纳只建集、不自动生产</span>
      }
    >
      <div className="grid lg:grid-cols-2 gap-5">
        {/* 左：对话 */}
        <div className="flex flex-col rounded-xl border border-ink-800 bg-ink-950/50 min-h-[380px]">
          <div ref={scrollRef} className="flex-1 overflow-y-auto p-4 space-y-3 max-h-[420px]">
            {!hasSession && (
              <div className="text-sm text-paper-300/50 leading-relaxed py-6">
                <p>和系列总编聊一聊这一季的构想：涵盖哪些人物与事件、叙事弧线、想要多少集。</p>
                <p className="mt-2">集数由主题体量决定——小切口几集即可，宏大主题可以规划数十集；随时可以让 AI 增删调整。</p>
                <Button
                  type="button"
                  variant="outline"
                  className="mt-4"
                  onClick={() => send(DEFAULT_PROMPT)}
                >
                  ✦ 开始规划
                </Button>
              </div>
            )}
            {messages.map((m, i) => (
              <div key={i} className={m.role === 'user' ? 'flex justify-end' : 'flex justify-start'}>
                <div
                  className={
                    m.role === 'user'
                      ? 'max-w-[85%] rounded-2xl rounded-tr-sm bg-gold-500/15 border border-gold-500/25 px-3.5 py-2 text-sm text-paper-100 whitespace-pre-wrap'
                      : 'max-w-[90%] rounded-2xl rounded-tl-sm bg-ink-800/80 border border-ink-700 px-3.5 py-2 text-sm text-paper-200/90 whitespace-pre-wrap'
                  }
                >
                  {m.content}
                </div>
              </div>
            ))}
            {chatMut.isPending && (
              <div className="flex items-center gap-2 text-xs text-gold-500/80">
                <Spinner className="w-3.5 h-3.5" /> 总编正在梳理史料、调整分集（约需数十秒）…
              </div>
            )}
          </div>
          <div className="border-t border-ink-800 p-3">
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  send(input)
                }
              }}
              rows={2}
              placeholder="描述你的调整，如：第 3 集换成张仪视角；再补 2 集收束合纵线"
              className={FIELD_CLS}
              disabled={chatMut.isPending}
            />
            <div className="flex items-center justify-between mt-2">
              <span className="text-xs text-paper-300/35">Enter 发送 · Shift+Enter 换行</span>
              <div className="flex gap-2">
                {hasSession && (
                  <Button
                    type="button"
                    variant="ghost"
                    disabled={resetMut.isPending || chatMut.isPending}
                    onClick={() => {
                      if (window.confirm('清空当前策划对话与草案，重新开始？（已采纳创建的集不受影响）')) {
                        resetMut.mutate()
                      }
                    }}
                  >
                    重新规划
                  </Button>
                )}
                <Button
                  type="button"
                  variant="seal"
                  disabled={!input.trim() || chatMut.isPending}
                  onClick={() => send(input)}
                >
                  {chatMut.isPending ? '思考中…' : '发送'}
                </Button>
              </div>
            </div>
          </div>
        </div>

        {/* 右：草案 */}
        <div className="flex flex-col rounded-xl border border-ink-800 bg-ink-950/50">
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-ink-800">
            <span className="text-sm text-paper-100">分集草案（{drafts.length}）</span>
            {drafts.length > 0 && (
              <span className="text-xs text-paper-300/45">待采纳 {pendingCount} 集 · 标题/主题/梗概均可手动修改</span>
            )}
          </div>
          <div className="flex-1 overflow-y-auto p-4 space-y-3 max-h-[360px]">
            {drafts.length === 0 && (
              <div className="text-sm text-paper-300/40 py-10 text-center">
                对话后这里会出现全量分集草案
              </div>
            )}
            {drafts.map((d, i) => {
              const created = existing.has(d.title.trim())
              return (
                <div
                  key={i}
                  className={`rounded-lg border px-3.5 py-3 space-y-2 ${
                    created ? 'border-ink-800 bg-ink-900/40 opacity-70' : 'border-ink-700 bg-ink-900/80'
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <span className="shrink-0 grid place-items-center w-7 h-7 rounded-md bg-ink-800 font-display text-xs text-gold-500">
                      {String(i + 1).padStart(2, '0')}
                    </span>
                    <input
                      value={d.title}
                      onChange={(e) => updateDraft(i, { title: e.target.value })}
                      className={`${FIELD_CLS} !py-1.5 font-display`}
                      placeholder="本集标题"
                    />
                    {created && (
                      <span className="shrink-0 text-[11px] text-emerald-300/80 border border-emerald-400/30 bg-emerald-400/10 rounded-full px-2 py-0.5">
                        已创建
                      </span>
                    )}
                    <button
                      type="button"
                      onClick={() => removeDraft(i)}
                      title="移除该草案"
                      className="shrink-0 rounded px-1.5 py-0.5 text-xs text-seal-500/70 hover:bg-seal-600/20 hover:text-seal-500"
                    >
                      ✕
                    </button>
                  </div>
                  <input
                    value={d.topic}
                    onChange={(e) => updateDraft(i, { topic: e.target.value })}
                    className={`${FIELD_CLS} !py-1.5`}
                    placeholder="主题 / 切入点（一句话）"
                  />
                  <textarea
                    value={d.summary}
                    onChange={(e) => updateDraft(i, { summary: e.target.value })}
                    rows={3}
                    className={`${FIELD_CLS} !py-1.5 leading-relaxed`}
                    placeholder="本集梗概（100-200 字）"
                  />
                </div>
              )
            })}
          </div>
          {/* 人物设定：跨集与分镜保持人物形象一致，随采纳一并保存 */}
          <div className="border-t border-ink-800">
            <div className="flex items-center justify-between px-4 py-2.5">
              <span className="text-sm text-paper-100">
                人物设定（{characters.length}）
                <span className="ml-2 text-xs text-paper-300/45">跨集与分镜形象一致 · 随采纳保存</span>
              </span>
              <Button type="button" variant="ghost" onClick={addCharacter}>
                + 人物
              </Button>
            </div>
            {characters.length > 0 && (
              <div className="px-4 pb-3 space-y-2 max-h-[240px] overflow-y-auto">
                {characters.map((c, i) => (
                  <div key={i} className="rounded-lg border border-ink-700 bg-ink-900/80 px-3.5 py-3 space-y-2">
                    <div className="flex items-center gap-2">
                      <input
                        value={c.name}
                        onChange={(e) => updateCharacter(i, { name: e.target.value })}
                        className={`${FIELD_CLS} !py-1.5 font-display w-28`}
                        placeholder="姓名"
                      />
                      <input
                        value={c.identity}
                        onChange={(e) => updateCharacter(i, { identity: e.target.value })}
                        className={`${FIELD_CLS} !py-1.5 flex-1`}
                        placeholder="身份，如：秦国相国，纵横家"
                      />
                      <button
                        type="button"
                        onClick={() => removeCharacter(i)}
                        title="移除该人物"
                        className="shrink-0 rounded px-1.5 py-0.5 text-xs text-seal-500/70 hover:bg-seal-600/20 hover:text-seal-500"
                      >
                        ✕
                      </button>
                    </div>
                    <input
                      value={c.appearance}
                      onChange={(e) => updateCharacter(i, { appearance: e.target.value })}
                      className={`${FIELD_CLS} !py-1.5`}
                      placeholder="外貌服饰固定描述（分镜将逐字复用，如：约四旬，清瘦挺拔，三缕短须，深青色深衣束发戴冠）"
                    />
                    <input
                      value={c.temperament}
                      onChange={(e) => updateCharacter(i, { temperament: e.target.value })}
                      className={`${FIELD_CLS} !py-1.5`}
                      placeholder="气质神态基调（如：沉毅多智，眉宇含锋）"
                    />
                  </div>
                ))}
              </div>
            )}
          </div>
          <div className="border-t border-ink-800 px-4 py-3 flex items-center justify-between gap-3">
            <div className="text-xs">
              {notice && <span className="text-emerald-300/90">{notice}</span>}
            </div>
            <Button
              type="button"
              variant="primary"
              disabled={applyMut.isPending || !canApply}
              onClick={() => {
                const parts: string[] = []
                if (pendingCount > 0) parts.push(`创建 ${pendingCount} 集`)
                if (characters.length > 0) parts.push(`保存 ${characters.length} 个人物设定`)
                if (window.confirm(`将${parts.join('，并')}（不自动生产视频，已创建的集自动跳过）。确定？`)) {
                  setNotice('')
                  applyMut.mutate()
                }
              }}
            >
              {applyMut.isPending && <Spinner className="w-3.5 h-3.5" />}
              采纳为集列表{pendingCount > 0 ? `（${pendingCount}）` : ''}
            </Button>
          </div>
        </div>
      </div>
      {err && <div className="mt-3"><ErrorBox>{err}</ErrorBox></div>}
    </Card>
  )
}

// 让消息更新时滚动到底部（内容变化也触发）。
function chatLoadingSentinel(messages: { content: string }[]): string {
  const last = messages[messages.length - 1]
  return last ? last.content : ''
}
