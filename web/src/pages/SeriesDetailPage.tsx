import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, seriesMediaUrl } from '../api'
import type {
  CharacterSetting,
  CreativeCatalog,
  CreativeKnob,
  CreativeStyle,
  Episode,
  PublishJob,
  Series,
} from '../types'
import {
  Button,
  Card,
  Drawer,
  Empty,
  ErrorBox,
  Field,
  Modal,
  Select,
  Spinner,
  StatusBadge,
  TextArea,
  TextInput,
} from '../components/ui'
import CreativeFields, { knobMap, knobValue } from '../components/CreativeFields'
import PlanPanel from '../components/PlanPanel'
import VoiceProfileCard from '../components/VoiceProfileCard'
import { STAGE_LABEL, activePath } from '../components/VersionTree'
import { useSeriesEvents } from '../useSeriesEvents'
import {
  PROVIDER_FIELDS,
  RATIO_OPTIONS,
  RESOLUTION_OPTIONS,
  formatTime,
  providerLabel,
  visualModeLabel,
} from '../labels'

const STAGE_ORDER = ['story', 'storyboard', 'media', 'final'] as const

// 一集在活跃路径上的推进情况（供集列表一眼看清：做到哪一步、有没有成片）。
type EpisodeState = {
  doneStages: number
  hasFinal: boolean
  failed: boolean
  running: boolean
  current: (typeof STAGE_ORDER)[number]
  status: 'pending' | 'running' | 'done' | 'failed'
}

function episodeState(ep: Episode): EpisodeState {
  const path = activePath(ep)
  const done = new Set<string>()
  let failed = false
  let running = false
  for (const n of path) {
    if (n.status === 'done') done.add(n.stage)
    if (n.status === 'failed') failed = true
    if (n.status === 'running') running = true
  }
  const last = path[path.length - 1]
  const finalNode = [...path].reverse().find((n) => n.stage === 'final')
  const hasFinal = !!finalNode && finalNode.status === 'done' && (finalNode.outputs?.length ?? 0) > 0
  return {
    doneStages: STAGE_ORDER.filter((s) => done.has(s)).length,
    hasFinal,
    failed,
    running,
    current: (last?.stage as EpisodeState['current']) ?? 'story',
    // 只有出了成片才算「已完成」；半途的集显示「未完成」，进度交给 n/4 阶段说明。
    status: running ? 'running' : failed ? 'failed' : hasFinal ? 'done' : 'pending',
  }
}

type FilterKey = 'all' | 'unfinished' | 'finished' | 'running' | 'failed'

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'unfinished', label: '未成片' },
  { key: 'finished', label: '已成片' },
  { key: 'running', label: '进行中' },
  { key: 'failed', label: '有失败' },
]

/** 某集发布任务的汇总展示（无任务时返回 null，不占位）。 */
function publishBadge(jobs: PublishJob[] | undefined) {
  if (!jobs || jobs.length === 0) return null
  const published = jobs.filter((j) => j.status === 'published').length
  if (published > 0) {
    return { text: `已发布 ${published}/${jobs.length}`, cls: 'text-emerald-300/80 border-emerald-400/30 bg-emerald-400/10' }
  }
  if (jobs.some((j) => j.status === 'failed' || j.status === 'rejected')) {
    return { text: '发布失败', cls: 'text-seal-500 border-seal-500/40 bg-seal-600/10' }
  }
  if (jobs.some((j) => j.status === 'uploading' || j.status === 'uploaded' || j.status === 'pending')) {
    return { text: '发布中', cls: 'text-gold-500 border-gold-500/40 bg-gold-500/10' }
  }
  return { text: '已取消', cls: 'text-paper-300/50 border-ink-700 bg-ink-900' }
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
  const [showPlan, setShowPlan] = useState(false)
  const [showDelete, setShowDelete] = useState(false)
  // 系列设置弹窗：集列表下方的配置内容全部收进这里，避免长列表把它们挤出视线。
  const [showSettings, setShowSettings] = useState(false)
  const [planNotice, setPlanNotice] = useState('')
  const [editing, setEditing] = useState<Episode | null>(null)
  const [filter, setFilter] = useState<FilterKey>('all')
  const [batchErr, setBatchErr] = useState('')
  const { job: seriesJob, running: seriesBusy } = useSeriesEvents(seriesId)

  // 声音名反查（折叠 badge 与设置区显示名字而非内部 ID）。
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
  // 系列级发布汇总：集列表按集显示发布状态。
  const { data: publishJobs } = useQuery({
    queryKey: ['series', seriesId, 'publish'],
    queryFn: () => api.listSeriesPublishJobs(seriesId),
    staleTime: 15_000,
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
  // 批量续跑：逐集串行触发「一键跑完整条流水线」（真实生成，按量计费）。
  const batchMut = useMutation({
    mutationFn: async (ids: string[]) => {
      for (const id of ids) await api.action(id, { action: 'run' })
    },
    onMutate: () => setBatchErr(''),
    onError: (e) => setBatchErr((e as Error).message),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['series', seriesId] }),
  })

  const jobsByEpisode = useMemo(() => {
    const m = new Map<string, PublishJob[]>()
    for (const j of publishJobs ?? []) {
      const list = m.get(j.episode_id) ?? []
      list.push(j)
      m.set(j.episode_id, list)
    }
    return m
  }, [publishJobs])

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

  const states = new Map(episodes.map((ep) => [ep.id, episodeState(ep)]))
  const toRun = episodes.filter((ep) => {
    const st = states.get(ep.id)!
    return !st.hasFinal && !st.running
  })
  const visible = episodes.filter((ep) => {
    const st = states.get(ep.id)!
    switch (filter) {
      case 'unfinished':
        return !st.hasFinal
      case 'finished':
        return st.hasFinal
      case 'running':
        return st.running
      case 'failed':
        return st.failed
      default:
        return true
    }
  })
  const finishedCount = episodes.filter((ep) => states.get(ep.id)!.hasFinal).length
  const runningCount = episodes.filter((ep) => states.get(ep.id)!.running).length
  const publishedEpisodes = new Set(
    (publishJobs ?? []).filter((j) => j.status === 'published').map((j) => j.episode_id),
  ).size
  const failedCount = episodes.filter((ep) => states.get(ep.id)!.failed).length

  return (
    <div className="space-y-5">
      <nav className="text-sm text-paper-300/45">
        <Link to="/" className="hover:text-gold-500">
          系列
        </Link>
        <span className="mx-2">/</span>
        <span className="text-paper-300/80">{s.name}</span>
      </nav>

      {/* 标题行：设置入口在标题行右侧（集列表下方不再堆配置区）；造集入口统一放在下方集列表的工具栏 */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
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
        <Button variant="outline" className="shrink-0" onClick={() => setShowSettings(true)}>
          ⚙ 系列设置
          {namedChars.length > 0 && (
            <span className="text-xs text-paper-300/45">
              {charsDone}/{namedChars.length} 参考图
            </span>
          )}
        </Button>
      </div>

      {/* 生产概览：一眼看清这个系列做到哪了 */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <InfoTile label="集数" value={`${episodes.length} 集`} />
        <InfoTile label="已成片" value={`${finishedCount} / ${episodes.length}`} />
        <InfoTile label="进行中" value={runningCount > 0 ? `${runningCount} 集` : '无'} />
        <InfoTile label="已发布" value={publishedEpisodes > 0 ? `${publishedEpisodes} 集` : '无'} />
      </div>

      {/* 集列表：页面主内容（配置类内容已收进标题行的设置弹窗）；两个造集入口（单个 / 批量规划）并列在工具栏 */}
      <Card
        title={`集列表（${episodes.length}）`}
        extra={
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button variant="outline" onClick={() => { setPlanNotice(''); setShowPlan(true) }}>
              ✦ AI 规划分集
            </Button>
            <Button variant="seal" onClick={() => setShowCreate(true)}>
              ＋ 新建一集
            </Button>
            {toRun.length > 0 && (
              <>
                <span className="hidden sm:block w-px h-5 bg-ink-700" />
                <Button
                  variant="outline"
                  disabled={batchMut.isPending}
                  onClick={() => {
                    if (
                      !window.confirm(
                        `将依次为 ${toRun.length} 集续跑整条流水线（生成故事→分镜→画面→成片）。这是真实生成，会按量产生费用，确定继续？`,
                      )
                    )
                      return
                    batchMut.mutate(toRun.map((e) => e.id))
                  }}
                >
                  {batchMut.isPending && <Spinner className="w-3.5 h-3.5" />}
                  续跑全部未成片（{toRun.length}）
                </Button>
              </>
            )}
            {failedCount > 0 && <span className="text-xs text-seal-500">{failedCount} 集有失败</span>}
          </div>
        }
      >
        {planNotice && (
          <div className="mb-3 flex items-start justify-between gap-3 rounded-lg border border-emerald-400/30 bg-emerald-400/10 px-4 py-2.5 text-sm text-emerald-300/90">
            <span>策划已采纳：{planNotice}</span>
            <button
              type="button"
              onClick={() => setPlanNotice('')}
              className="shrink-0 text-xs text-emerald-300/60 hover:text-emerald-300"
            >
              关闭
            </button>
          </div>
        )}
        {batchErr && (
          <div className="mb-3">
            <ErrorBox>{batchErr}</ErrorBox>
          </div>
        )}
        {episodes.length === 0 ? (
          <Empty text="该系列还没有集：用「＋ 新建一集」单集创建，或用「✦ AI 规划分集」一次规划整季" />
        ) : (
          <>
            <div className="mb-3 flex flex-wrap items-center gap-1.5">
              {FILTERS.map((f) => (
                <button
                  key={f.key}
                  type="button"
                  onClick={() => setFilter(f.key)}
                  className={`rounded-full border px-3 py-1 text-xs transition-colors ${
                    filter === f.key
                      ? 'border-gold-500/60 bg-gold-500/10 text-gold-500'
                      : 'border-ink-700 text-paper-300/60 hover:text-paper-100'
                  }`}
                >
                  {f.label}
                </button>
              ))}
            </div>
            {visible.length === 0 ? (
              <Empty text="当前筛选下没有集" />
            ) : (
              <ul className="divide-y divide-ink-800">
                {visible.map((ep) => {
                  const st = states.get(ep.id)!
                  const pub = publishBadge(jobsByEpisode.get(ep.id))
                  return (
                    <li
                      key={ep.id}
                      className="flex items-center gap-3 py-3.5 px-2 -mx-2 rounded-lg hover:bg-ink-800/60 transition-colors"
                    >
                      <Link
                        to={`/series/${s.id}/episodes/${ep.id}`}
                        className="flex items-center gap-4 min-w-0 flex-1"
                      >
                        <span className="shrink-0 grid place-items-center w-10 h-10 rounded-lg bg-ink-800 font-display text-gold-500">
                          E{String(ep.number).padStart(2, '0')}
                        </span>
                        <div className="min-w-0 flex-1">
                          <div className="text-paper-100 truncate">{ep.title}</div>
                          <div className="mt-0.5 text-xs text-paper-300/45 truncate">
                            {ep.topic || '未填主题'}
                            <span className="mx-1.5 text-paper-300/25">·</span>
                            更新于 {formatTime(ep.updated_at)}
                          </div>
                        </div>
                      </Link>
                      <div className="shrink-0 flex items-center gap-2.5">
                        {pub && (
                          <span className={`hidden sm:inline rounded-full border px-2 py-0.5 text-[11px] ${pub.cls}`}>
                            {pub.text}
                          </span>
                        )}
                        <span
                          className="hidden md:inline text-xs text-paper-300/40"
                          title={`已完成 ${st.doneStages}/4 个阶段`}
                        >
                          {st.doneStages}/4 阶段
                        </span>
                        <span className="text-xs text-paper-300/40 hidden sm:inline">{STAGE_LABEL[st.current]}</span>
                        {st.status === 'pending' ? (
                          <span className="inline-flex items-center gap-1.5 rounded-full border border-ink-700 bg-ink-900 px-2.5 py-0.5 text-xs text-paper-300/50">
                            <span className="w-1.5 h-1.5 rounded-full bg-paper-300/30" />
                            未完成
                          </span>
                        ) : (
                          <StatusBadge status={st.status} />
                        )}
                        <button
                          type="button"
                          onClick={() => setEditing(ep)}
                          title="编辑本集信息"
                          className="rounded px-1.5 py-0.5 text-xs text-paper-300/60 hover:text-paper-100 hover:bg-ink-800"
                        >
                          编辑
                        </button>
                        <button
                          type="button"
                          onClick={() => {
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
                      </div>
                    </li>
                  )
                })}
              </ul>
            )}
          </>
        )}
      </Card>

      <CreateEpisodeModal seriesId={s.id} open={showCreate} onClose={() => setShowCreate(false)} />
      {/* AI 分集策划：批量造集入口，与集列表同一工具栏；满高抽屉承载对话 + 草案，不占详情页纵向空间 */}
      <Drawer
        open={showPlan}
        onClose={() => setShowPlan(false)}
        title="AI 分集策划"
        extra={
          <span className="text-xs text-paper-300/45 truncate">
            对话持久保存 · 采纳只建集、不自动生产
          </span>
        }
      >
        <PlanPanel
          seriesId={s.id}
          existingTitles={episodes.map((e) => e.title)}
          onApplied={(notice) => {
            setPlanNotice(notice)
            setShowPlan(false)
          }}
        />
      </Drawer>
      <EpisodeEditModal
        episode={editing}
        onClose={() => setEditing(null)}
        onSaved={() => void queryClient.invalidateQueries({ queryKey: ['series', seriesId] })}
      />
      {/* 系列设置弹窗（标题行入口）：基础信息/规格/创作 + 声音 + 视觉参考 + 删除，集中一处 */}
      <Modal
        open={showSettings}
        onClose={() => setShowSettings(false)}
        title="系列设置"
        maxWidth="max-w-4xl"
      >
        <div className="space-y-6">
          <section>
            <div className="text-xs text-paper-300/40 mb-3">基础信息 / 规格 / 创作</div>
            <SeriesSettingsCard
              series={s}
              catalog={catalog}
              voiceName={voiceName}
              voiceLoading={!voicesData}
            />
          </section>

          <section className="border-t border-ink-800 pt-5">
            <div className="text-xs text-paper-300/40 mb-3">
              声音（创建后锁定）
              <span className="ml-2 text-paper-300/30">{voiceName || '未关联'}</span>
            </div>
            <VoiceProfileCard series={s} />
          </section>

          <section className="border-t border-ink-800 pt-5">
            <div className="text-xs text-paper-300/40 mb-3">
              系列视觉参考 · 人物
              {namedChars.length > 0 && (
                <span className="ml-2 text-paper-300/30">
                  {charsDone}/{namedChars.length} 已生成
                </span>
              )}
            </div>
            <CharactersCard seriesId={s.id} characters={namedChars} busy={seriesBusy} job={seriesJob} />
          </section>

          {/* 删除系列（需输入系列名确认）：放在弹窗末尾，低调处理 */}
          <section className="border-t border-ink-800 pt-5 flex justify-end">
            <button
              type="button"
              onClick={() => setShowDelete(true)}
              className="text-xs text-seal-500/50 hover:text-seal-500 hover:underline"
            >
              删除系列
            </button>
          </section>
        </div>
      </Modal>
      <DeleteSeriesModal
        series={s}
        open={showDelete}
        pending={deleteSeriesMut.isPending}
        onClose={() => setShowDelete(false)}
        onConfirm={() => deleteSeriesMut.mutate()}
      />
    </div>
  )
}

/** 系列设置卡：基础信息与规格（PUT /api/series/{id}）+ 创作控制参数（PUT /creative）。 */
function SeriesSettingsCard({
  series,
  catalog,
  voiceName,
  voiceLoading,
}: {
  series: Series
  catalog?: CreativeCatalog
  voiceName?: string
  voiceLoading: boolean
}) {
  return (
    <div className="space-y-6">
      <SeriesMetaSection series={series} voiceName={voiceName} voiceLoading={voiceLoading} />
      <div className="border-t border-ink-800 pt-6">
        <CreativeSection series={series} catalog={catalog} />
      </div>
    </div>
  )
}

type MetaDraft = {
  name: string
  dynasty: string
  description: string
  ratio: string
  resolution: string
  max_concurrency: number
  max_retries: number
  text_provider: string
  tts_provider: string
  image_provider: string
  video_provider: string
}

function metaDraftOf(s: Series): MetaDraft {
  return {
    name: s.name,
    dynasty: s.dynasty ?? '',
    description: s.description ?? '',
    ratio: s.config.ratio,
    resolution: s.config.resolution,
    max_concurrency: s.config.max_concurrency,
    max_retries: s.config.max_retries,
    text_provider: s.config.text_provider ?? '',
    tts_provider: s.config.tts_provider ?? '',
    image_provider: s.config.image_provider ?? '',
    video_provider: s.config.video_provider ?? '',
  }
}

/** 基础信息与规格：只读展示 ⇄ 编辑态整体保存（补丁接口，只提交本区块字段）。 */
function SeriesMetaSection({
  series,
  voiceName,
  voiceLoading,
}: {
  series: Series
  voiceName?: string
  voiceLoading: boolean
}) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<MetaDraft>(() => metaDraftOf(series))
  const [err, setErr] = useState('')

  const mutation = useMutation({
    mutationFn: (v: MetaDraft) => api.updateSeries(series.id, { ...v, name: v.name.trim() }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series', series.id] })
      setEditing(false)
      setErr('')
    },
    onError: (e) => setErr((e as Error).message),
  })

  if (editing) {
    const set = <K extends keyof MetaDraft>(k: K, val: MetaDraft[K]) => setDraft((d) => ({ ...d, [k]: val }))
    return (
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          setErr('')
          mutation.mutate(draft)
        }}
      >
        <div className="grid sm:grid-cols-2 gap-4">
          <Field label="系列名称（必填）">
            <TextInput value={draft.name} onChange={(e) => set('name', e.target.value)} />
          </Field>
          <Field label="朝代锚定">
            <TextInput value={draft.dynasty} onChange={(e) => set('dynasty', e.target.value)} placeholder="如：战国" />
          </Field>
          <div className="sm:col-span-2">
            <Field label="简介">
              <TextArea
                value={draft.description}
                maxLength={300}
                onChange={(e) => set('description', e.target.value)}
                placeholder="一两句话说明这个系列讲什么"
              />
            </Field>
          </div>
          <Field label="画面比例">
            <Select value={draft.ratio} onChange={(e) => set('ratio', e.target.value)}>
              {RATIO_OPTIONS.map((r) => (
                <option key={r}>{r}</option>
              ))}
            </Select>
          </Field>
          <Field label="分辨率（档位为长边）">
            <Select value={draft.resolution} onChange={(e) => set('resolution', e.target.value)}>
              {RESOLUTION_OPTIONS.map((r) => (
                <option key={r}>{r}</option>
              ))}
            </Select>
          </Field>
          <Field label="最大并发镜头数">
            <TextInput
              type="number"
              min={1}
              value={draft.max_concurrency}
              onChange={(e) => set('max_concurrency', Number(e.target.value) || 1)}
            />
          </Field>
          <Field label="单镜最大重试次数">
            <TextInput
              type="number"
              min={0}
              value={draft.max_retries}
              onChange={(e) => set('max_retries', Number(e.target.value) || 0)}
            />
          </Field>
          {PROVIDER_FIELDS.map((f) => (
            <Field key={f.key} label={`${f.label} Provider`}>
              <Select value={draft[f.key]} onChange={(e) => set(f.key, e.target.value)}>
                {f.options.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </Select>
            </Field>
          ))}
        </div>
        <p className="text-xs text-paper-300/40">
          画面模式（{visualModeLabel(series.config.visual_mode)}）与声音在创建时锁定，不能在此修改。
        </p>
        {err && <ErrorBox>{err}</ErrorBox>}
        <div className="flex gap-3">
          <Button type="submit" variant="seal" disabled={mutation.isPending || !draft.name.trim()}>
            {mutation.isPending && <Spinner />} 保存
          </Button>
          <Button
            type="button"
            variant="ghost"
            disabled={mutation.isPending}
            onClick={() => {
              setDraft(metaDraftOf(series))
              setEditing(false)
              setErr('')
            }}
          >
            取消
          </Button>
        </div>
      </form>
    )
  }

  return (
    <div>
      <div className="text-xs text-paper-300/40 mb-3">基础信息与规格</div>
      <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-4 text-sm">
        <InfoTile label="名称" value={series.name} />
        <InfoTile label="朝代" value={series.dynasty || '未设置'} />
        <InfoTile label="画幅 / 分辨率" value={`${series.config.ratio} · ${series.config.resolution}`} />
        <InfoTile label="并发 / 重试" value={`${series.config.max_concurrency} 路 · ${series.config.max_retries} 次`} />
        <InfoTile label="画面模式（锁定）" value={visualModeLabel(series.config.visual_mode)} />
        <InfoTile
          label="声音（锁定）"
          value={voiceLoading ? '读取中…' : voiceName || '未关联'}
        />
        {PROVIDER_FIELDS.map((f) => (
          <InfoTile key={f.key} label={`${f.label} Provider`} value={providerLabel(f.key, series.config[f.key])} />
        ))}
      </div>
      {series.description && (
        <p className="mt-4 text-sm text-paper-300/60 leading-relaxed">{series.description}</p>
      )}
      <div className="mt-3 flex items-center gap-4 text-xs text-paper-300/35">
        <span>创建于 {formatTime(series.created_at)}</span>
        <span>更新于 {formatTime(series.updated_at)}</span>
      </div>
      <div className="mt-4">
        <Button variant="outline" onClick={() => setEditing(true)}>
          编辑基础信息
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

/**
 * 创作参数区：只读展示当前参数的中文值；进入编辑态内嵌 CreativeFields，整体保存。
 * 参数的 key/选项一律读自 catalog，不在此硬编码。
 */
function CreativeSection({ series, catalog }: { series: Series; catalog?: CreativeCatalog }) {
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
      return api.updateCreative(series.id, {
        preset: values.preset || catalog.default_preset,
        creative: knobMap(values, catalog),
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series', series.id] })
      setEditing(false)
      setErr('')
    },
    onError: (e) => setErr((e as Error).message),
  })

  // 费用相关参数：由注册表的 cost_impact 显式声明（后端新增费用参数后此处自动生效）。
  const costChanged = (catalog?.knobs ?? []).filter(
    (k) => k.cost_impact && knobValue(draft, k.key) !== knobValue(current, k.key),
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
        <div className="text-xs text-paper-300/40">创作控制参数</div>
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
      <div className="text-xs text-paper-300/40 mb-3">创作控制参数</div>
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
          编辑创作参数
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
      <div className="mt-1 text-paper-100 truncate" title={value}>
        {value}
      </div>
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
        <p className="text-xs text-paper-300/40">创建集本身不产生费用；之后在集详情页运行流水线才会计费。</p>
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

/** 编辑集信息（标题/主题/附加指令）：只改元数据，不触碰版本树与产物。 */
function EpisodeEditModal({
  episode,
  onClose,
  onSaved,
}: {
  episode: Episode | null
  onClose: () => void
  onSaved: () => void
}) {
  const [title, setTitle] = useState('')
  const [topic, setTopic] = useState('')
  const [instruction, setInstruction] = useState('')
  const [err, setErr] = useState('')
  const [loadedID, setLoadedID] = useState<string | null>(null)

  // 打开另一集时把表单同步成该集当前值（同一集保持用户正在输入的内容）。
  if (episode && loadedID !== episode.id) {
    setLoadedID(episode.id)
    setTitle(episode.title)
    setTopic(episode.topic)
    setInstruction(episode.instruction ?? '')
    setErr('')
  }

  const mutation = useMutation({
    mutationFn: () => api.updateEpisode(episode!.id, { title: title.trim(), topic: topic.trim(), instruction: instruction.trim() }),
    onSuccess: () => {
      onSaved()
      setLoadedID(null)
      onClose()
    },
    onError: (e) => setErr((e as Error).message),
  })

  return (
    <Modal open={!!episode} onClose={onClose} title={episode ? `编辑 E${String(episode.number).padStart(2, '0')}` : ''}>
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          setErr('')
          mutation.mutate()
        }}
      >
        <Field label="本集标题（必填）">
          <TextInput value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </Field>
        <Field label="主题 / 切入点（可空）">
          <TextInput value={topic} onChange={(e) => setTopic(e.target.value)} />
        </Field>
        <Field label="本集附加指令（可空，叠加在系列创作设置之上）">
          <TextArea value={instruction} onChange={(e) => setInstruction(e.target.value)} maxLength={500} />
        </Field>
        <p className="text-xs text-paper-300/40">
          只修改文字信息，不影响已生成的版本与产物；改动会在下次运行时生效。
        </p>
        {err && <ErrorBox>{err}</ErrorBox>}
        <div className="flex gap-3">
          <Button type="submit" variant="seal" disabled={mutation.isPending || !title.trim()}>
            {mutation.isPending && <Spinner />} 保存
          </Button>
          <Button type="button" variant="ghost" onClick={onClose}>
            取消
          </Button>
        </div>
      </form>
    </Modal>
  )
}

/** 删除系列：要求逐字输入系列名，避免误删（连同全部集与产物）。 */
function DeleteSeriesModal({
  series,
  open,
  pending,
  onClose,
  onConfirm,
}: {
  series: Series
  open: boolean
  pending: boolean
  onClose: () => void
  onConfirm: () => void
}) {
  const [typed, setTyped] = useState('')
  const matched = typed.trim() === series.name

  return (
    <Modal
      open={open}
      onClose={() => {
        setTyped('')
        onClose()
      }}
      title="删除系列"
    >
      <div className="space-y-4">
        <p className="text-sm text-paper-300/70 leading-relaxed">
          将删除系列「{series.name}」及其全部集与产物，此操作不可撤销。
          请输入系列名以确认。
        </p>
        <Field label={`输入「${series.name}」以确认`}>
          <TextInput value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus />
        </Field>
        <div className="flex gap-3">
          <Button
            variant="seal"
            disabled={!matched || pending}
            onClick={() => {
              onConfirm()
              setTyped('')
            }}
          >
            {pending && <Spinner />} 确认删除
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              setTyped('')
              onClose()
            }}
          >
            取消
          </Button>
        </div>
      </div>
    </Modal>
  )
}