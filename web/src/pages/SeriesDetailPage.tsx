import { useState, type MouseEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, seriesMediaUrl } from '../api'
import type { CharacterSetting, CreativeCatalog, CreativeKnob, CreativeStyle, Episode, Series } from '../types'
import { Button, Card, Collapsible, Empty, ErrorBox, Field, Modal, Spinner, TextArea, TextInput } from '../components/ui'
import { StatusBadge } from '../components/ui'
import CreativeFields, { knobMap, knobValue } from '../components/CreativeFields'
import PlanPanel from '../components/PlanPanel'
import VoiceProfileCard from '../components/VoiceProfileCard'
import { STAGE_LABEL, activePath } from '../components/VersionTree'
import { useSeriesEvents } from '../useSeriesEvents'

// §17 版本树：集进度取活跃路径上最深的已执行节点。
function episodeProgress(ep: Episode) {
  const path = activePath(ep)
  for (let i = path.length - 1; i >= 0; i--) {
    if (path[i].status !== 'pending') {
      return { step: path[i].stage, status: path[i].status }
    }
  }
  return path.length
    ? { step: path[0].stage, status: path[0].status }
    : { step: 'story' as const, status: 'pending' as const }
}

function visualModeLabel(mode: string | undefined): string {
  return mode === 'video' ? 'AI 视频' : '小人书插画'
}

export default function SeriesDetailPage() {
  const { seriesId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({
    queryKey: ['series', seriesId],
    queryFn: () => api.getSeries(seriesId),
  })
  const [showCreate, setShowCreate] = useState(false)
  const { job: seriesJob, running: seriesBusy } = useSeriesEvents(seriesId)
  // 声音名反查（折叠 badge 显示名字而非内部 ID）。
  const { data: voicesData } = useQuery({
    queryKey: ['voices'],
    queryFn: api.listVoices,
    staleTime: 60_000,
  })
  // 创作参数注册表：翻译参数 key → 中文名、渲染编辑控件都靠它。
  const { data: catalog } = useQuery({
    queryKey: ['creative-catalog'],
    queryFn: api.getCreativeCatalog,
    staleTime: Infinity,
  })

  const deleteSeriesMut = useMutation({
    mutationFn: () => api.deleteSeries(seriesId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      navigate('/')
    },
  })
  const deleteEpisodeMut = useMutation({
    mutationFn: (id: string) => api.deleteEpisode(id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['series', seriesId] }),
  })

  if (isLoading) {
    return (
      <div className="flex justify-center py-20">
        <Spinner className="w-6 h-6" />
      </div>
    )
  }
  if (error) return <ErrorBox>{(error as Error).message}</ErrorBox>
  if (!data) return null

  const { series: s, episodes } = data
  const voiceName = voicesData?.find((v) => v.id === s.voice_id)?.name
  const namedChars = (s.characters ?? []).filter((c) => c.name.trim())
  const charsDone = namedChars.filter((c) => c.ref_image).length

  return (
    <div className="space-y-5">
      <nav className="text-sm text-paper-300/45">
        <Link to="/" className="hover:text-gold-500">
          系列
        </Link>
        <span className="mx-2">/</span>
        <span className="text-paper-300/80">{s.name}</span>
      </nav>

      {/* 标题行 */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-3xl tracking-wider flex items-center gap-3">
            {s.name}
            {s.dynasty && (
              <span className="text-sm rounded border border-seal-500/40 bg-seal-600/10 px-2 py-0.5 text-seal-500 font-body">
                {s.dynasty}
              </span>
            )}
            <span className="text-xs rounded border border-ink-700 bg-ink-900 px-2 py-0.5 text-paper-300/60 font-body">
              {visualModeLabel(s.config.visual_mode)}
            </span>
          </h1>
          {s.description && <p className="mt-2 text-sm text-paper-300/60 max-w-2xl">{s.description}</p>}
        </div>
        <Button variant="seal" onClick={() => setShowCreate(true)}>
          ＋ 新建一集
        </Button>
      </div>

      {/* 系列信息（折叠） */}
      <Collapsible summary="系列信息" badge={<span className="text-xs text-paper-300/40">{episodes.length} 集</span>}>
        <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-4 text-sm">
          <InfoTile label="画面模式" value={visualModeLabel(s.config.visual_mode)} />
          <InfoTile label="画幅 / 分辨率" value={`${s.config.ratio} · ${s.config.resolution}`} />
          <InfoTile label="TTS 音色" value={s.config.tts_voice} />
          <InfoTile label="并发 / 重试" value={`${s.config.max_concurrency} 路 · ${s.config.max_retries} 次`} />
        </div>
        {s.config.tts_instruction && (
          <div className="mt-4">
            <div className="text-xs text-paper-300/40 mb-1">旁白风格指令</div>
            <p className="text-sm text-paper-300/70 leading-relaxed">{s.config.tts_instruction}</p>
          </div>
        )}
      </Collapsible>

      {/* 声音（§16：顶层实体，创建后锁定） */}
      <Collapsible summary="声音" badge={<span className="text-xs text-paper-300/40">{voiceName || '未关联'}</span>}>
        <VoiceProfileCard seriesId={s.id} />
      </Collapsible>

      {/* AI 分集策划（折叠，默认收起） */}
      <Collapsible summary="AI 分集策划" badge={<span className="text-xs text-paper-300/40">对话式批量建集</span>}>
        <PlanPanel seriesId={s.id} existingTitles={episodes.map((e) => e.title)} />
      </Collapsible>

      {/* 系列视觉参考（人物，跨集复用；折叠，默认收起，标题显示进度） */}
      <Collapsible
        summary="系列视觉参考 · 人物"
        badge={
          namedChars.length > 0 ? (
            <span className="text-xs text-paper-300/45">
              {charsDone}/{namedChars.length} 已生成
            </span>
          ) : undefined
        }
      >
        <CharactersCard seriesId={s.id} characters={namedChars} busy={seriesBusy} job={seriesJob} />
      </Collapsible>

      {/* 创作控制参数（折叠，默认收起）：不同用户/系列可产出不同风格 */}
      <Collapsible summary="创作设置" badge={<span className="text-xs text-paper-300/40">叙事 / 受众 / 篇幅 / 画风</span>}>
        <CreativeCard seriesId={s.id} series={s} catalog={catalog} />
      </Collapsible>

      {/* 集列表（始终展示，主要内容） */}
      <Card title={`集列表（${episodes.length}）`}>
        {episodes.length === 0 ? (
          <Empty text="该系列还没有集，点击右上角「新建一集」开始" />
        ) : (
          <ul className="divide-y divide-ink-800">
            {episodes.map((ep) => {
              const p = episodeProgress(ep)
              return (
                <li key={ep.id}>
                  <Link
                    to={`/series/${s.id}/episodes/${ep.id}`}
                    className="flex items-center gap-4 py-3.5 px-2 -mx-2 rounded-lg hover:bg-ink-800/60 transition-colors"
                  >
                    <span className="shrink-0 grid place-items-center w-10 h-10 rounded-lg bg-ink-800 font-display text-gold-500">
                      E{String(ep.number).padStart(2, '0')}
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="text-paper-100 truncate">{ep.title}</div>
                      {ep.topic && <div className="text-xs text-paper-300/45 truncate mt-0.5">{ep.topic}</div>}
                    </div>
                    <div className="shrink-0 flex items-center gap-3">
                      <span className="text-xs text-paper-300/40 hidden sm:inline">{STAGE_LABEL[p.step]}</span>
                      <StatusBadge status={p.status} />
                      <button
                        type="button"
                        onClick={(e: MouseEvent) => {
                          e.preventDefault()
                          e.stopPropagation()
                          if (window.confirm(`确定删除本集「${ep.title}」及其所有产物？此操作不可撤销。`)) {
                            deleteEpisodeMut.mutate(ep.id)
                          }
                        }}
                        disabled={deleteEpisodeMut.isPending}
                        title="删除本集"
                        className="rounded px-1.5 py-0.5 text-xs text-seal-500/70 opacity-60 hover:opacity-100 hover:bg-seal-600/20"
                      >
                        ✕
                      </button>
                      <span className="text-paper-300/30">›</span>
                    </div>
                  </Link>
                </li>
              )
            })}
          </ul>
        )}
      </Card>

      {/* 删除系列（下沉到页脚，低调处理） */}
      <div className="flex justify-end pt-2">
        <button
          type="button"
          onClick={() => {
            if (window.confirm(`确定删除系列「${s.name}」及其全部集与产物？此操作不可撤销。`)) {
              deleteSeriesMut.mutate()
            }
          }}
          disabled={deleteSeriesMut.isPending}
          className="text-xs text-seal-500/50 hover:text-seal-500 hover:underline"
        >
          {deleteSeriesMut.isPending ? '删除中…' : '删除系列'}
        </button>
      </div>

      {/* 新建一集弹窗 */}
      <CreateEpisodeModal seriesId={s.id} open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  )
}

/**
 * 创作设置卡片：只读展示当前参数的中文值；进入编辑态内嵌 CreativeFields，整体保存。
 * 参数的 key/选项一律读自 catalog，不在此硬编码。
 */
function CreativeCard({
  seriesId,
  series,
  catalog,
}: {
  seriesId: string
  series: Series
  catalog?: CreativeCatalog
}) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<CreativeStyle>({})
  const [err, setErr] = useState('')

  // 当前值：config.creative 各参数 + 画风（画风在后端是独立字段 config.video_style）。
  const current: CreativeStyle = { ...(series.config.creative ?? {}), video_style: series.config.video_style }

  const mutation = useMutation({
    mutationFn: (values: CreativeStyle) => {
      if (!catalog) throw new Error('创作参数注册表尚未加载')
      // 整体覆盖写：把全部 knob 值（含空串＝清除）一次提交，避免只交差异造成回填歧义。
      // 预设留空时回落到注册表的默认预设，显式清掉旧的溯源 key。
      return api.updateCreative(seriesId, {
        preset: values.preset || catalog.default_preset,
        creative: knobMap(values, catalog),
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series', seriesId] })
      setEditing(false)
      setErr('')
    },
    onError: (e) => setErr((e as Error).message),
  })

  // 费用相关参数：注册表 help 里点名「费用」的项（靠 help 识别，不在前端硬编码 key，
  // 后端新增/调整费用参数后此处自动生效）。
  const costChanged = (catalog?.knobs ?? []).filter(
    (k) => k.help.includes('费用') && knobValue(draft, k.key) !== knobValue(current, k.key),
  )

  if (!catalog) return <div className="text-sm text-paper-300/40 py-2">创作设置加载中…</div>

  const startEdit = () => {
    setDraft(current)
    setErr('')
    setEditing(true)
  }

  const save = () => {
    if (costChanged.length > 0) {
      const names = costChanged.map((k) => k.label).join('、')
      if (!window.confirm(`改动「${names}」会重做分镜 / 重出插画并产生费用，确定保存？`)) return
    }
    mutation.mutate(draft)
  }

  if (editing) {
    return (
      <div className="space-y-4">
        <CreativeFields catalog={catalog} values={draft} onChange={setDraft} disabled={mutation.isPending} />
        {costChanged.length > 0 && (
          <div className="rounded-lg border border-gold-500/40 bg-gold-500/10 px-4 py-3 text-sm text-gold-500">
            改动「{costChanged.map((k) => k.label).join('、')}」会使分镜与画面重做，保存后将产生费用。
          </div>
        )}
        {err && <ErrorBox>{err}</ErrorBox>}
        <div className="flex gap-3">
          <Button variant="seal" disabled={mutation.isPending} onClick={save}>
            {mutation.isPending && <Spinner />} 保存
          </Button>
          <Button
            variant="ghost"
            disabled={mutation.isPending}
            onClick={() => {
              setEditing(false)
              setErr('')
            }}
          >
            取消
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div>
      {current.preset && (
        <p className="mb-3 text-xs text-paper-300/50">
          预设：{catalog.presets.find((p) => p.key === current.preset)?.name ?? current.preset}
        </p>
      )}
      <div className="grid sm:grid-cols-2 gap-4 text-sm">
        {catalog.knobs.map((k) => (
          <div key={k.key}>
            <div className="text-xs text-paper-300/40 mb-1">{k.label}</div>
            <div className="text-paper-100">{displayKnobValue(k, current)}</div>
          </div>
        ))}
      </div>
      <div className="mt-4">
        <Button variant="outline" onClick={startEdit}>
          编辑
        </Button>
      </div>
      {err && (
        <div className="mt-3">
          <ErrorBox>{err}</ErrorBox>
        </div>
      )}
    </div>
  )
}

/** 把参数值翻译成中文：空值＝跟随默认；枚举回译选项名；文本原样。 */
function displayKnobValue(knob: CreativeKnob, values: CreativeStyle): string {
  const v = knobValue(values, knob.key)
  if (!v) return '跟随默认'
  return knob.options.find((o) => o.key === v)?.label ?? v
}

/** 系列视觉参考卡片（人物，跨集复用）：预览 + 生成/重新生成参考图。 */
function CharactersCard({
  seriesId,
  characters,
  busy,
  job,
}: {
  seriesId: string
  characters: CharacterSetting[]
  busy: boolean
  job: import('../types').JobEvent | null
}) {
  const [err, setErr] = useState('')

  const keyframesMut = useMutation({
    mutationFn: (force: boolean) => api.generateKeyframes(seriesId, force),
    onSuccess: () => setErr(''),
    onError: (e) => setErr((e as Error).message),
  })

  const doneCount = characters.filter((c) => c.ref_image).length

  return (
    <>
      {characters.length === 0 ? (
        <Empty text="暂无人物设定——在 AI 分集策划中让 AI 产出，或手动添加后采纳。单元剧的单集人物与场景类参考在各集详情页管理。" />
      ) : (
        <>
          <div className="grid sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {characters.map((c) => (
              <div
                key={c.name}
                className="rounded-xl border border-ink-800 bg-ink-950/60 overflow-hidden flex flex-col"
              >
                <div className="aspect-[3/4] bg-ink-900 grid place-items-center overflow-hidden">
                  {c.ref_image ? (
                    <img
                      src={seriesMediaUrl(seriesId, c.ref_image)}
                      alt={`${c.name} 视觉参考图`}
                      className="w-full h-full object-contain"
                      loading="lazy"
                    />
                  ) : (
                    <span className="text-4xl font-display text-paper-300/20">{c.name.slice(0, 1)}</span>
                  )}
                </div>
                <div className="p-3 space-y-1.5">
                  <div className="flex items-center gap-2">
                    <span className="font-display text-paper-100">{c.name}</span>
                    <span
                      className={`text-[11px] rounded-full px-2 py-0.5 border ${
                        c.ref_image
                          ? 'text-emerald-300/80 border-emerald-400/30 bg-emerald-400/10'
                          : 'text-paper-300/50 border-ink-700 bg-ink-900'
                      }`}
                    >
                      {c.ref_image ? '已生成' : '未生成'}
                    </span>
                  </div>
                  {c.identity && <div className="text-xs text-gold-500/80">{c.identity}</div>}
                  {c.appearance && (
                    <p className="text-xs text-paper-300/55 leading-relaxed line-clamp-3" title={c.appearance}>
                      {c.appearance}
                    </p>
                  )}
                  {c.temperament && (
                    <p className="text-xs text-paper-300/40 leading-relaxed line-clamp-2" title={c.temperament}>
                      气质：{c.temperament}
                    </p>
                  )}
                </div>
              </div>
            ))}
          </div>

          <div className="mt-4 flex items-center gap-3">
            <Button
              type="button"
              variant="seal"
              disabled={busy || keyframesMut.isPending}
              onClick={() => keyframesMut.mutate(false)}
            >
              {busy || keyframesMut.isPending ? <Spinner className="w-3.5 h-3.5" /> : null}
              {doneCount < characters.length ? '生成缺失参考图（按张计费）' : '全部已生成'}
            </Button>
            <Button
              type="button"
              variant="ghost"
              disabled={busy || keyframesMut.isPending}
              onClick={() => {
                if (window.confirm('重新生成全部参考图？已有图片将被覆盖，并再次产生图片费用。')) {
                  keyframesMut.mutate(true)
                }
              }}
            >
              重新生成全部
            </Button>
            {job?.status === 'running' && (
              <span className="text-xs text-gold-500/80 flex items-center gap-1.5">
                <Spinner className="w-3 h-3" /> 正在生成参考图…
              </span>
            )}
            {job?.status === 'failed' && <span className="text-xs text-seal-500">上次生成失败：{job.error}</span>}
          </div>
        </>
      )}
      {err && (
        <div className="mt-3">
          <ErrorBox>{err}</ErrorBox>
        </div>
      )}
    </>
  )
}

function InfoTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-ink-800 bg-ink-900/70 px-4 py-3">
      <div className="text-xs text-paper-300/40">{label}</div>
      <div className="mt-1 text-paper-100">{value}</div>
    </div>
  )
}

/** 新建一集弹窗。 */
function CreateEpisodeModal({
  seriesId,
  open,
  onClose,
}: {
  seriesId: string
  open: boolean
  onClose: () => void
}) {
  const [title, setTitle] = useState('')
  const [topic, setTopic] = useState('')
  // 本集附加指令：叠加在系列创作设置之上，可空（空则不提交该字段）。
  const [instruction, setInstruction] = useState('')
  const [err, setErr] = useState('')
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: () =>
      api.createEpisode(seriesId, {
        title: title.trim(),
        topic: topic.trim(),
        ...(instruction.trim() ? { instruction: instruction.trim() } : {}),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series', seriesId] })
      setTitle('')
      setTopic('')
      setInstruction('')
      setErr('')
      onClose()
    },
    onError: (e) => setErr((e as Error).message),
  })

  return (
    <Modal open={open} onClose={onClose} title="新建一集">
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          setErr('')
          mutation.mutate()
        }}
      >
        <Field label="本集标题（必填）">
          <TextInput value={title} onChange={(e) => setTitle(e.target.value)} placeholder="如：入秦" autoFocus />
        </Field>
        <Field label="主题 / 切入点（可空，AI 自由命题）">
          <TextInput value={topic} onChange={(e) => setTopic(e.target.value)} placeholder="如：苏秦张仪出山之前" />
        </Field>
        <Field label="本集附加指令（可空，叠加在系列创作设置之上）">
          <TextArea
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            maxLength={500}
            placeholder="如：这一集只讲一个夜晚的故事"
          />
        </Field>
        {err && <ErrorBox>{err}</ErrorBox>}
        <div className="flex gap-3">
          <Button type="submit" variant="seal" disabled={mutation.isPending || !title.trim()}>
            {mutation.isPending && <Spinner />} 创建
          </Button>
          <Button type="button" variant="ghost" onClick={onClose}>
            取消
          </Button>
        </div>
      </form>
    </Modal>
  )
}
