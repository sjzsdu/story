import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, mediaUrl } from '../api'
import type { ActionName, Episode, JobEvent, MediaResult, Scene, StoryCandidate } from '../types'
import { episodeQueryKey, useEpisodeEvents } from '../useEpisodeEvents'
import { Button, Card, Empty, ErrorBox, Spinner } from '../components/ui'
import StepsBar from '../components/StepsBar'

const ACTION_LABEL: Record<string, string> = {
  candidates: '生成候选故事',
  pick: '选定故事',
  storyboard: '拆分分镜',
  produce: '生产视频与旁白',
  compose: '合成成片',
  run: '一键跑完整条流水线',
  export: '导出其他比例',
}

export default function EpisodePage() {
  const { seriesId = '', episodeId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data: ep, isLoading, error } = useQuery({
    queryKey: episodeQueryKey(episodeId),
    queryFn: () => api.getEpisode(episodeId),
  })
  const { job, running } = useEpisodeEvents(episodeId)
  const [actionErr, setActionErr] = useState('')

  const action = useMutation({
    mutationFn: (body: { action: ActionName; index?: number; ratio?: string }) => api.action(episodeId, body),
    onMutate: () => setActionErr(''),
    onError: (e) => setActionErr((e as Error).message),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: episodeQueryKey(episodeId) }),
  })

  const deleteMut = useMutation({
    mutationFn: () => api.deleteEpisode(episodeId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series', seriesId] })
      navigate(`/series/${seriesId}`)
    },
  })

  if (isLoading) {
    return (
      <div className="flex justify-center py-20">
        <Spinner className="w-6 h-6" />
      </div>
    )
  }
  if (error) return <ErrorBox>{(error as Error).message}</ErrorBox>
  if (!ep) return null

  const st = ep.state
  const act = (a: ActionName, extra?: { index?: number; ratio?: string }) =>
    action.mutate({ action: a, ...extra })

  return (
    <div className="space-y-6">
      <nav className="text-sm text-paper-300/45 flex items-center gap-2 flex-wrap">
        <Link to="/" className="hover:text-gold-500">
          系列
        </Link>
        <span>/</span>
        <Link to={`/series/${seriesId}`} className="hover:text-gold-500">
          {ep.series_id}
        </Link>
        <span>/</span>
        <span className="text-paper-300/80">
          E{String(ep.number).padStart(2, '0')} {ep.title}
        </span>
      </nav>

      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="font-display text-3xl tracking-wider">
            <span className="text-gold-500 mr-3">E{String(ep.number).padStart(2, '0')}</span>
            {ep.title}
          </h1>
          {ep.topic && <p className="mt-2 text-sm text-paper-300/55">主题：{ep.topic}</p>}
        </div>
        <Button
          variant="ghost"
          onClick={() => {
            if (window.confirm(`确定删除本集「${ep.title}」及其所有产物？此操作不可撤销。`)) {
              deleteMut.mutate()
            }
          }}
          disabled={deleteMut.isPending}
        >
          {deleteMut.isPending ? '删除中…' : '删除本集'}
        </Button>
      </div>

      {running && <JobBanner job={job} />}
      {!running && job?.status === 'failed' && <ErrorBox>{ACTION_LABEL[job.action] ?? job.action}失败：{job.error}</ErrorBox>}
      {actionErr && <ErrorBox>{actionErr}</ErrorBox>}

      <Card title="流水线">
        <div className="flex flex-col gap-5">
          <StepsBar state={st} />
          <StepErrors ep={ep} />
          <Toolbar ep={ep} busy={running} onAction={act} pending={action.isPending} />
        </div>
      </Card>

      <CandidatesSection ep={ep} busy={running} onPick={(i) => act('pick', { index: i })} pending={action.isPending} />
      <StorySection story={st.story} />
      <StoryboardSection ep={ep} />
      <OutputsSection ep={ep} busy={running} onExport={(ratio) => act('export', { ratio })} pending={action.isPending} />
    </div>
  )
}

function JobBanner({ job }: { job: JobEvent | null }) {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-gold-500/40 bg-gold-500/10 px-4 py-3">
      <Spinner />
      <span className="text-sm text-gold-500">
        正在执行：{job ? ACTION_LABEL[job.action] ?? job.action : '任务'}…
      </span>
      <span className="text-xs text-paper-300/45">状态每秒自动刷新，可离开本页，进度会断点续跑</span>
    </div>
  )
}

function StepErrors({ ep }: { ep: Episode }) {
  const failed = Object.entries(ep.state.steps).filter(([, v]) => v.status === 'failed' && v.error)
  if (failed.length === 0) return null
  return (
    <div className="space-y-2">
      {failed.map(([name, v]) => (
        <div key={name} className="text-sm">
          <span className="text-seal-500 font-medium">「{ACTION_LABEL[name] ?? name}」错误：</span>
          <span className="text-paper-300/75">{v.error}</span>
        </div>
      ))}
    </div>
  )
}

function Toolbar({
  ep,
  busy,
  pending,
  onAction,
}: {
  ep: Episode
  busy: boolean
  pending: boolean
  onAction: (a: ActionName, extra?: { index?: number; ratio?: string }) => void
}) {
  const s = ep.state.steps
  const needsIndex = !ep.state.story
  const [runIndex, setRunIndex] = useState(1)
  return (
    <div className="flex flex-wrap gap-2.5 items-center">
      <Button variant="seal" disabled={busy || pending} onClick={() => onAction('candidates')}>
        {ep.state.candidates?.length ? '重新生成候选' : '① 生成候选故事'}
      </Button>
      <Button variant="primary" disabled={busy || pending} onClick={() => onAction('storyboard')}>
        ③ 拆分分镜
      </Button>
      <Button variant="primary" disabled={busy || pending} onClick={() => onAction('produce')}>
        ④ 生产片段与旁白
      </Button>
      <Button variant="primary" disabled={busy || pending} onClick={() => onAction('compose')}>
        ⑤ 合成成片
      </Button>
      {needsIndex && (
        <label className="flex items-center gap-1.5 text-xs text-paper-300/55">
          候选序号
          <input
            type="number"
            min={1}
            value={runIndex}
            onChange={(e) => setRunIndex(Math.max(1, Number(e.target.value) || 1))}
            className="w-14 rounded border border-ink-700 bg-ink-950 px-2 py-1 text-paper-100"
          />
        </label>
      )}
      <Button
        variant="outline"
        disabled={busy || pending}
        onClick={() => onAction('run', needsIndex ? { index: runIndex } : undefined)}
      >
        ⚡ 一键跑到底
      </Button>
      <span className="self-center text-xs text-paper-300/35">
        已完成的步骤会自动跳过；② 选定请在下方候选卡片操作
      </span>
      {s.compose.status === 'done' && null}
    </div>
  )
}

function CandidatesSection({
  ep,
  busy,
  pending,
  onPick,
}: {
  ep: Episode
  busy: boolean
  pending: boolean
  onPick: (index: number) => void
}) {
  const cs = ep.state.candidates
  if (!cs || cs.length === 0) return null
  const selected = ep.state.selected
  return (
    <Card title={`候选故事（${cs.length}）${selected ? ` · 已选 #${selected}` : ''}`}>
      <div className="grid md:grid-cols-2 gap-4">
        {cs.map((c) => (
          <CandidateCard key={c.index} c={c} selected={selected === c.index} busy={busy} pending={pending} onPick={onPick} />
        ))}
      </div>
    </Card>
  )
}

function CandidateCard({
  c,
  selected,
  busy,
  pending,
  onPick,
}: {
  c: StoryCandidate
  selected: boolean
  busy: boolean
  pending: boolean
  onPick: (index: number) => void
}) {
  const [open, setOpen] = useState(false)
  return (
    <div
      className={`rounded-xl border p-4 flex flex-col gap-2.5 ${
        selected ? 'border-gold-500/70 bg-gold-500/5' : 'border-ink-700 bg-ink-950/50'
      }`}
    >
      <div className="flex items-start justify-between gap-3">
        <h4 className="font-display text-lg text-paper-100">
          <span className="text-gold-500 mr-2">[{c.index}]</span>
          {c.title}
        </h4>
        {selected && <span className="text-xs text-gold-500 shrink-0">已选定</span>}
      </div>
      <div className="text-xs text-paper-300/50 flex flex-wrap gap-x-4 gap-y-1">
        {c.dynasty && <span>朝代：{c.dynasty}</span>}
        {c.source && <span>出处：{c.source}</span>}
      </div>
      <p className="text-sm text-paper-300/75 leading-relaxed">{c.summary}</p>
      <button className="self-start text-xs text-paper-300/45 hover:text-gold-500" onClick={() => setOpen((v) => !v)}>
        {open ? '收起正文' : '展开正文'}
      </button>
      {open && (
        <div className="rounded-lg bg-ink-900/80 border border-ink-800 p-3.5 text-sm leading-7 text-paper-300/85 whitespace-pre-wrap max-h-80 overflow-auto">
          {c.content}
        </div>
      )}
      {!selected && (
        <Button variant="outline" className="self-start" disabled={busy || pending} onClick={() => onPick(c.index)}>
          ② 选定 #{c.index}
        </Button>
      )}
    </div>
  )
}

function StorySection({ story }: { story?: StoryCandidate }) {
  if (!story) return null
  return (
    <Card
      title={
        <span>
          《{story.title}》
          <span className="ml-3 text-xs text-paper-300/45 font-body">
            {story.dynasty} · {story.source}
          </span>
        </span>
      }
      extra={<span className="text-xs text-paper-300/35">工作目录 story.md 可人工修改</span>}
    >
      <div className="rounded-lg bg-ink-950/50 border border-ink-800 p-5 text-[15px] leading-8 text-paper-300/90 whitespace-pre-wrap font-display">
        {story.content}
      </div>
    </Card>
  )
}

function StoryboardSection({ ep }: { ep: Episode }) {
  const sb = ep.state.storyboard
  if (!sb) return null
  const clipById = new Map<number, MediaResult>(ep.state.clips?.map((m) => [m.scene_id, m]) ?? [])
  const audioById = new Map<number, MediaResult>(ep.state.audios?.map((m) => [m.scene_id, m]) ?? [])

  return (
    <Card title={`分镜脚本（${sb.scenes.length} 镜）`}>
      <ol className="space-y-4">
        {sb.scenes.map((sc) => (
          <SceneCard key={sc.id} ep={ep} scene={sc} clip={clipById.get(sc.id)} audio={audioById.get(sc.id)} />
        ))}
      </ol>
    </Card>
  )
}

function SceneCard({
  ep,
  scene: sc,
  clip,
  audio,
}: {
  ep: Episode
  scene: Scene
  clip?: MediaResult
  audio?: MediaResult
}) {
  return (
    <li className="rounded-xl border border-ink-800 bg-ink-950/40 overflow-hidden">
      <div className="flex flex-wrap items-center gap-2 px-4 py-2.5 border-b border-ink-800 bg-ink-900/60">
        <span className="font-display text-gold-500">镜 {String(sc.id).padStart(2, '0')}</span>
        <span className="text-xs rounded bg-ink-800 px-2 py-0.5 text-paper-300/60">{sc.duration}s</span>
        {sc.camera && <span className="text-xs text-paper-300/50">运镜：{sc.camera}</span>}
        {clip?.skipped && <span className="text-xs text-emerald-300/70">复用已有片段</span>}
        {clip?.err && <span className="text-xs text-seal-500">片段失败：{clip.err}</span>}
      </div>
      <div className="grid lg:grid-cols-[1fr_300px] gap-4 p-4">
        <div className="space-y-3 min-w-0">
          <div>
            <div className="text-xs text-paper-300/40 mb-1">画面（已做朝代视觉锚定）</div>
            <p className="text-sm leading-7 text-paper-300/85">{sc.visual_prompt}</p>
          </div>
          <div>
            <div className="text-xs text-paper-300/40 mb-1">旁白</div>
            <p className="text-sm leading-7 text-paper-100/90">{sc.narration}</p>
          </div>
        </div>
        <div className="space-y-3">
          {clip?.path ? (
            <video
              key={clip.path}
              controls
              preload="metadata"
              className="w-full rounded-lg border border-ink-700 bg-black aspect-[9/16] object-contain"
              src={mediaUrl(ep.id, clip.path)}
            />
          ) : (
            <div className="w-full aspect-[9/16] rounded-lg border border-dashed border-ink-700 grid place-items-center text-xs text-paper-300/30">
              视频片段未生产
            </div>
          )}
          {audio?.path ? (
            <audio controls preload="none" className="w-full h-9" src={mediaUrl(ep.id, audio.path)} />
          ) : (
            <div className="text-xs text-paper-300/30 text-center">旁白未生产</div>
          )}
        </div>
      </div>
    </li>
  )
}

function OutputsSection({
  ep,
  busy,
  pending,
  onExport,
}: {
  ep: Episode
  busy: boolean
  pending: boolean
  onExport: (ratio: string) => void
}) {
  const outs = ep.state.outputs ?? []
  const ratios = ['16:9', '9:16', '1:1', '3:4']
  return (
    <Card
      title="成片与多比例导出"
      extra={
        <div className="flex items-center gap-2">
          <span className="text-xs text-paper-300/40 mr-1">导出：</span>
          {ratios.map((r) => (
            <Button key={r} variant="outline" className="px-2.5 py-1 text-xs" disabled={busy || pending} onClick={() => onExport(r)}>
              {r}
            </Button>
          ))}
        </div>
      }
    >
      {outs.length === 0 ? (
        <Empty text="尚无成片。完成分镜后依次执行 ④ 生产 → ⑤ 合成" />
      ) : (
        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {outs.map((p) => (
            <div key={p} className="space-y-2">
              <video
                controls
                preload="metadata"
                className="w-full rounded-lg border border-ink-700 bg-black max-h-[520px] object-contain"
                src={mediaUrl(ep.id, p)}
              />
              <a
                href={mediaUrl(ep.id, p)}
                download
                className="block text-xs text-paper-300/45 hover:text-gold-500 truncate"
                title={p}
              >
                ⬇ {p.split('/').pop()}
              </a>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
