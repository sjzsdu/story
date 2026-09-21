import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, mediaUrl } from '../api'
import type { ActionName, Episode, JobEvent, MediaResult, Scene, StoryCandidate, VisualRef } from '../types'
import { episodeQueryKey, useEpisodeEvents } from '../useEpisodeEvents'
import { Button, Card, Collapsible, Empty, ErrorBox, Spinner } from '../components/ui'
import StepsBar from '../components/StepsBar'

const ACTION_LABEL: Record<string, string> = {
  candidates: '生成故事',
  pick: '选定故事',
  storyboard: '拆分分镜',
  produce: '生产画面与旁白',
  compose: '合成成片',
  run: '一键跑完整条流水线',
  export: '导出其他比例',
  'episode-refs': '生成本集视觉参考图',
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
    mutationFn: (body: { action: ActionName; index?: number; ratio?: string; scenes?: number[] }) =>
      api.action(episodeId, body),
    onMutate: () => setActionErr(''),
    onError: (e) => setActionErr((e as Error).message),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: episodeQueryKey(episodeId) }),
  })

  const cancelMut = useMutation({
    mutationFn: () => api.cancelEpisode(episodeId),
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
  const act = (a: ActionName, extra?: { index?: number; ratio?: string; scenes?: number[] }) =>
    action.mutate({ action: a, ...extra })
  const missing = missingScenes(ep)

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

      {running && <JobBanner job={job} stopping={cancelMut.isPending} onStop={() => cancelMut.mutate()} />}
      {!running && job?.status === 'canceled' && (
        <div className="rounded-xl border border-ink-600 bg-ink-900/70 px-4 py-3 text-sm text-paper-300/80">
          已手动停止「{ACTION_LABEL[job.action] ?? job.action}」。已完成的产物全部保留，再次执行会从断点续跑。
        </div>
      )}
      {!running && job?.status === 'failed' && <ErrorBox>{ACTION_LABEL[job.action] ?? job.action}失败：{job.error}</ErrorBox>}
      {actionErr && <ErrorBox>{actionErr}</ErrorBox>}
      {!running && missing.length > 0 && <MissingBanner ep={ep} missing={missing} onRetry={() => act('produce')} pending={action.isPending} />}

      <Card title="流水线">
        <div className="flex flex-col gap-5">
          <StepsBar state={st} />
          <StepErrors ep={ep} />
          <Toolbar ep={ep} missing={missing} busy={running} onAction={act} pending={action.isPending} />
        </div>
      </Card>

      <StorySection story={st.story} busy={running} pending={action.isPending} onRegenerate={() => act('candidates')} />
      <EpisodeRefsSection ep={ep} busy={running} />
      <StoryboardSection ep={ep} busy={running} pending={action.isPending} onRetryScene={(id) => act('produce', { scenes: [id] })} />
      <OutputsSection ep={ep} busy={running} onExport={(ratio) => act('export', { ratio })} pending={action.isPending} />
    </div>
  )
}

/** 未完成镜头：画面或旁白任一缺失（含生成失败）的镜头号列表。 */
function missingScenes(ep: Episode): number[] {
  const sb = ep.state.storyboard
  if (!sb) return []
  const clipById = new Map(ep.state.clips?.map((m) => [m.scene_id, m]) ?? [])
  const audioById = new Map(ep.state.audios?.map((m) => [m.scene_id, m]) ?? [])
  return sb.scenes
    .filter((sc) => !clipById.get(sc.id)?.path || !audioById.get(sc.id)?.path)
    .map((sc) => sc.id)
}

function formatScenes(ids: number[]): string {
  return ids.map((id) => `#${String(id).padStart(2, '0')}`).join('、')
}

function JobBanner({
  job,
  stopping,
  onStop,
}: {
  job: JobEvent | null
  stopping: boolean
  onStop: () => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border border-gold-500/40 bg-gold-500/10 px-4 py-3">
      <Spinner />
      <span className="text-sm text-gold-500">
        正在执行：{job ? ACTION_LABEL[job.action] ?? job.action : '任务'}…
      </span>
      <span className="text-xs text-paper-300/45">状态每秒自动刷新，可离开本页，进度会断点续跑</span>
      <Button
        variant="outline"
        className="ml-auto px-3 py-1.5 text-xs"
        disabled={stopping}
        onClick={() => {
          if (window.confirm('停止后会 kill 正在执行的生成任务；已完成的产物全部保留，之后可续跑。确定停止？')) {
            onStop()
          }
        }}
      >
        {stopping ? '停止中…' : '停止'}
      </Button>
    </div>
  )
}

/** 未完成镜头汇总：一键只重试这些镜头，已成功的镜头不会被重复生成。 */
function MissingBanner({
  ep,
  missing,
  onRetry,
  pending,
}: {
  ep: Episode
  missing: number[]
  onRetry: () => void
  pending: boolean
}) {
  const clipErr = new Map(
    (ep.state.clips ?? []).filter((m) => m.err).map((m) => [m.scene_id, m.err as string]),
  )
  const audioErr = new Map(
    (ep.state.audios ?? []).filter((m) => m.err).map((m) => [m.scene_id, m.err as string]),
  )
  return (
    <div className="rounded-xl border border-seal-500/40 bg-seal-600/10 px-4 py-3 space-y-2">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm text-seal-500 font-medium">
          {missing.length} 个镜头未完成：{formatScenes(missing)}
        </span>
        <Button variant="seal" className="ml-auto px-3 py-1.5 text-xs" disabled={pending} onClick={onRetry}>
          仅重试失败镜头（{missing.length} 镜）
        </Button>
      </div>
      <ul className="space-y-1 text-xs text-paper-300/70">
        {missing.map((id) => (
          <li key={id}>
            <span className="text-paper-300/50">镜 {String(id).padStart(2, '0')}：</span>
            {[
              clipErr.get(id) && `画面 — ${clipErr.get(id)}`,
              audioErr.get(id) && `旁白 — ${audioErr.get(id)}`,
            ]
              .filter(Boolean)
              .join('；') || '尚未生成（上次执行未跑到或被中断）'}
          </li>
        ))}
      </ul>
      <p className="text-xs text-paper-300/35">
        重试只会触碰上面这些镜头；已成功的镜头与旁白直接复用，不会重复出图/合成。
      </p>
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
  missing,
  busy,
  pending,
  onAction,
}: {
  ep: Episode
  missing: number[]
  busy: boolean
  pending: boolean
  onAction: (a: ActionName, extra?: { index?: number; ratio?: string; scenes?: number[] }) => void
}) {
  const produceLabel =
    missing.length > 0 ? `③ 仅重试失败镜头（${missing.length} 镜）` : '③ 生产画面与旁白'
  return (
    <div className="flex flex-wrap gap-2.5 items-center">
      <Button variant="seal" disabled={busy || pending} onClick={() => onAction('candidates')}>
        {ep.state.story ? '① 重新生成故事' : '① 生成故事'}
      </Button>
      <Button variant="primary" disabled={busy || pending} onClick={() => onAction('storyboard')}>
        ② 拆分分镜
      </Button>
      <Button variant="primary" disabled={busy || pending} onClick={() => onAction('produce')}>
        {produceLabel}
      </Button>
      <Button variant="primary" disabled={busy || pending} onClick={() => onAction('compose')}>
        ④ 合成成片
      </Button>
      <Button variant="outline" disabled={busy || pending} onClick={() => onAction('run')}>
        ⚡ 一键跑到底
      </Button>
      <span className="self-center text-xs text-paper-300/35">
        生产时已完成的镜头（画面/旁白）会自动复用跳过，不会重复出图或合成
      </span>
    </div>
  )
}

function StorySection({
  story,
  busy,
  pending,
  onRegenerate,
}: {
  story?: StoryCandidate
  busy: boolean
  pending: boolean
  onRegenerate: () => void
}) {
  if (!story) {
    return (
      <Card>
        <Empty text="还没有故事。点击「① 生成故事」，AI 直接产出一篇定稿，无需在候选间选择。" />
      </Card>
    )
  }
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
      extra={
        <div className="flex items-center gap-3">
          <span className="text-xs text-paper-300/35">工作目录 story.md 可人工修改</span>
          <Button variant="outline" className="px-2.5 py-1 text-xs" disabled={busy || pending} onClick={onRegenerate}>
            重新生成
          </Button>
        </div>
      }
    >
      <p className="mb-3 text-sm text-paper-300/60 leading-relaxed">{story.summary}</p>
      <div className="rounded-lg bg-ink-950/50 border border-ink-800 p-5 text-[15px] leading-8 text-paper-300/90 whitespace-pre-wrap font-display">
        {story.content}
      </div>
    </Card>
  )
}

function StoryboardSection({
  ep,
  busy,
  pending,
  onRetryScene,
}: {
  ep: Episode
  busy: boolean
  pending: boolean
  onRetryScene: (id: number) => void
}) {
  const sb = ep.state.storyboard
  if (!sb) return null
  const clipById = new Map<number, MediaResult>(ep.state.clips?.map((m) => [m.scene_id, m]) ?? [])
  const audioById = new Map<number, MediaResult>(ep.state.audios?.map((m) => [m.scene_id, m]) ?? [])

  return (
    <Card title={`分镜脚本（${sb.scenes.length} 镜）`}>
      <ol className="space-y-4">
        {sb.scenes.map((sc) => (
          <SceneCard
            key={sc.id}
            ep={ep}
            scene={sc}
            clip={clipById.get(sc.id)}
            audio={audioById.get(sc.id)}
            busy={busy}
            pending={pending}
            onRetry={() => onRetryScene(sc.id)}
          />
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
  busy,
  pending,
  onRetry,
}: {
  ep: Episode
  scene: Scene
  clip?: MediaResult
  audio?: MediaResult
  busy: boolean
  pending: boolean
  onRetry: () => void
}) {
  const incomplete = !clip?.path || !audio?.path
  const reuseAll = clip?.skipped && audio?.skipped
  return (
    <li>
      <Collapsible
        summary={
          <span className="flex items-center gap-2">
            <span className="font-display text-gold-500">镜 {String(sc.id).padStart(2, '0')}</span>
            <span className="text-xs rounded bg-ink-800 px-2 py-0.5 text-paper-300/60">{sc.duration}s</span>
            {sc.camera && <span className="text-xs text-paper-300/50">运镜：{sc.camera}</span>}
            {reuseAll && <span className="text-xs text-emerald-300/70">复用</span>}
            {incomplete && <span className="text-xs text-seal-500">未完成</span>}
          </span>
        }
        defaultOpen
      >
        <div className="grid lg:grid-cols-[1fr_300px] gap-4 p-4 border-t border-ink-800">
          <div className="space-y-3 min-w-0">
            <div>
              <div className="text-xs text-paper-300/40 mb-1">画面（已做朝代视觉锚定）</div>
              <p className="text-sm leading-7 text-paper-300/85">{sc.visual_prompt}</p>
            </div>
            <div>
              <div className="text-xs text-paper-300/40 mb-1">旁白</div>
              <p className="text-sm leading-7 text-paper-100/90">{sc.narration}</p>
            </div>
            {incomplete && (
              <div className="flex flex-wrap items-center gap-2.5">
                <Button
                  variant="seal"
                  className="px-3 py-1.5 text-xs"
                  disabled={busy || pending}
                  onClick={onRetry}
                >
                  重试本镜
                </Button>
                <span className="text-xs text-paper-300/40">
                  只重跑这一镜未完成的部分，已生成的那一项直接复用
                </span>
              </div>
            )}
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
            {clip?.err && <ErrorBox>画面：{clip.err}</ErrorBox>}
            {audio?.path ? (
              <audio controls preload="none" className="w-full h-9" src={mediaUrl(ep.id, audio.path)} />
            ) : (
              <div className="text-xs text-paper-300/30 text-center">旁白未生产</div>
            )}
            {audio?.err && <ErrorBox>旁白：{audio.err}</ErrorBox>}
          </div>
        </div>
      </Collapsible>
    </li>
  )
}

function EpisodeRefsSection({ ep, busy }: { ep: Episode; busy: boolean }) {
  const queryClient = useQueryClient()
  const [errMsg, setErrMsg] = useState('')
  const refs = ep.refs ?? []
  const characters = refs.filter((r) => r.kind !== 'scene')
  const scenes = refs.filter((r) => r.kind === 'scene')
  const withImage = refs.filter((r) => r.ref_image).length

  const gen = useMutation({
    mutationFn: (force: boolean) => api.generateEpisodeRefs(ep.id, force),
    onMutate: () => setErrMsg(''),
    onError: (e) => setErrMsg((e as Error).message),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: episodeQueryKey(ep.id) }),
  })

  const hasStoryboard = !!ep.state.storyboard
  const badge = (
    <span className="text-xs text-paper-300/40">
      {refs.length} 条参考 · {withImage} 张图
    </span>
  )

  return (
    <Collapsible
      summary="本集视觉参考（人物 / 场景一致性约束）"
      badge={badge}
      defaultOpen={refs.length > 0}
    >
      <div className="px-5 py-4 space-y-4 border-t border-ink-800">
        <div className="flex flex-wrap items-center gap-2.5">
          <Button
            variant="seal"
            className="px-3 py-1.5 text-xs"
            disabled={busy || gen.isPending || refs.length === 0}
            onClick={() => gen.mutate(false)}
          >
            {gen.isPending ? '生成中…' : '生成缺失的参考图（按张计费）'}
          </Button>
          <Button
            variant="outline"
            className="px-3 py-1.5 text-xs"
            disabled={busy || gen.isPending || refs.length === 0}
            onClick={() => {
              if (window.confirm('将重新生成全部参考图并产生图片费用，确定？')) gen.mutate(true)
            }}
          >
            全部重新生成
          </Button>
          <span className="text-xs text-paper-300/35">
            文字约束在分镜阶段已零成本生效；参考图仅 video 模式生产时喂给视频模型，comic 模式以文字约束为准
          </span>
        </div>
        {errMsg && <ErrorBox>{errMsg}</ErrorBox>}
        {refs.length === 0 ? (
          <Empty
            text={
              hasStoryboard
                ? '本分镜为旧版数据，未产出视觉参考。重新执行一次「② 拆分分镜」即可获得人物/场景的文字约束（不产生图片费用）。'
                : '拆分分镜后，这里会列出本集人物与重复出现的场景（如兰若寺大殿），用于跨镜头一致性约束。'
            }
          />
        ) : (
          <>
            <RefGroup title="人物" refs={characters} ep={ep} />
            <RefGroup title="场景" refs={scenes} ep={ep} />
          </>
        )}
      </div>
    </Collapsible>
  )
}

function RefGroup({ title, refs, ep }: { title: string; refs: VisualRef[]; ep: Episode }) {
  if (refs.length === 0) return null
  return (
    <div>
      <div className="text-xs text-gold-500/80 mb-2 tracking-wider">{title}</div>
      <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {refs.map((r) => (
          <div key={`${r.kind}-${r.name}`} className="rounded-lg border border-ink-800 bg-ink-950/40 p-3 flex gap-3">
            {r.ref_image ? (
              <img
                src={mediaUrl(ep.id, r.ref_image)}
                alt={r.name}
                className="w-16 h-24 shrink-0 rounded object-cover border border-ink-700"
                loading="lazy"
              />
            ) : (
              <div className="w-16 h-24 shrink-0 rounded border border-dashed border-ink-700 grid place-items-center text-[10px] text-paper-300/30 text-center px-1">
                参考图
                <br />
                未生成
              </div>
            )}
            <div className="min-w-0">
              <div className="font-display text-sm text-paper-100">{r.name}</div>
              <p className="mt-1 text-xs leading-5 text-paper-300/60 line-clamp-5">{r.description}</p>
            </div>
          </div>
        ))}
      </div>
    </div>
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
