import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, mediaUrl } from '../api'
import type {
  ActionName,
  Episode,
  JobEvent,
  MediaResult,
  Scene,
  Storyboard,
  StoryCandidate,
  VersionNode,
  VisualRef,
} from '../types'
import { episodeQueryKey, useEpisodeEvents } from '../useEpisodeEvents'
import { Button, Card, Collapsible, Empty, ErrorBox, Spinner, StatusBadge } from '../components/ui'
import VersionTree, { STAGE_LABEL, activePath } from '../components/VersionTree'

const ACTION_LABEL: Record<string, string> = {
  story: '生成故事',
  storyboard: '拆分分镜',
  produce: '生产画面与旁白',
  compose: '合成成片',
  run: '一键跑完整条流水线',
  export: '导出其他比例',
  'episode-refs': '生成本集视觉参考图',
}

// ActExtra 是流水线动作的附加参数：from 指定作用节点，reroll 开新版本，
// note 是「本版附加要求」（重做时填的迭代方向），ratio/scenes 用于导出与单镜重跑。
type ActExtra = { from?: string; reroll?: boolean; note?: string; ratio?: string; scenes?: number[] }

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
  const [selectedId, setSelectedId] = useState('')

  const refresh = () => void queryClient.invalidateQueries({ queryKey: episodeQueryKey(episodeId) })

  const action = useMutation({
    mutationFn: (body: { action: ActionName } & ActExtra) => api.action(episodeId, body),
    onMutate: () => setActionErr(''),
    onError: (e) => setActionErr((e as Error).message),
    onSettled: refresh,
  })

  const nodeMut = useMutation({
    mutationFn: (v: { kind: 'activate' | 'delete'; nodeId: string }) =>
      v.kind === 'activate' ? api.activateNode(episodeId, v.nodeId) : api.deleteNode(episodeId, v.nodeId),
    onMutate: () => setActionErr(''),
    onError: (e) => setActionErr((e as Error).message),
    onSuccess: (_ep, v) => {
      if (v.kind === 'delete' && v.nodeId === selectedId) setSelectedId('')
    },
    onSettled: refresh,
  })

  const cancelMut = useMutation({
    mutationFn: () => api.cancelEpisode(episodeId),
    onMutate: () => setActionErr(''),
    onError: (e) => setActionErr((e as Error).message),
    onSettled: refresh,
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

  const act = (a: ActionName, extra?: ActExtra) => action.mutate({ action: a, ...extra })
  const path = activePath(ep)
  const activeNode = path[path.length - 1] ?? null
  // 默认选中活跃主线末端；活跃指针缺失时退到最后一个节点，避免「有版本却说没有」。
  const selected = ep.nodes.find((n) => n.id === selectedId) ?? activeNode ?? ep.nodes[ep.nodes.length - 1] ?? null
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
          {ep.instruction && (
            <p className="mt-1 text-sm text-paper-300/55">本集附加指令：{ep.instruction}</p>
          )}
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
      {!running && missing.length > 0 && <MissingBanner ep={ep} missing={missing} />}

      <Card title="版本树" extra={<span className="text-xs text-paper-300/40">每一步的产物按派生键独立成版本；上游一变即自动失效，旧版本完整保留</span>}>
        <div className="flex flex-col gap-5">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-paper-300/40">
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-0.5 w-4 rounded bg-gold-500" />
              金色连线＝当前主线
            </span>
            <span>点卡片＝选中它，卡片上会出现只针对它的操作</span>
          </div>
          {ep.nodes.length === 0 ? (
            <div className="space-y-3 rounded-lg border border-dashed border-ink-700 px-5 py-8 text-center">
              <p className="text-sm text-paper-300/50">
                还没有任何版本。先产出一篇定稿口播稿，后面每一步都从它派生。
              </p>
              <Button variant="primary" disabled={running || action.isPending} onClick={() => act('story')}>
                ▶ 开始：生成故事
              </Button>
            </div>
          ) : (
            <VersionTree
              ep={ep}
              selectedId={selected?.id ?? ''}
              busy={running}
              pending={action.isPending || nodeMut.isPending}
              onSelect={setSelectedId}
              onAction={act}
              onActivate={(id) => nodeMut.mutate({ kind: 'activate', nodeId: id })}
              onDelete={(id) => nodeMut.mutate({ kind: 'delete', nodeId: id })}
            />
          )}
        </div>
      </Card>

      {selected && (
        <NodeDetail
          ep={ep}
          node={selected}
          busy={running}
          pending={action.isPending}
          onAction={act}
        />
      )}

      <EpisodeRefsSection ep={ep} busy={running} />
    </div>
  )
}

/** activeNodeOfStage 返回活跃路径上指定阶段的节点。 */
function activeNodeOfStage(ep: Episode, stage: VersionNode['stage']): VersionNode | null {
  const path = activePath(ep)
  for (let i = path.length - 1; i >= 0; i--) {
    if (path[i].stage === stage) return path[i]
  }
  return null
}

/**
 * missingScenes 列出活跃画面节点中画面或旁白缺失（含失败）的镜头号。
 * 只统计活跃路径——「生产画面」默认就以活跃路径为准，二者语义一致。
 */
function missingScenes(ep: Episode): number[] {
  const media = activeNodeOfStage(ep, 'media')
  if (!media) return []
  const board = ep.nodes.find((n) => n.id === media.parent_id)
  if (!board?.storyboard) return []
  const clipById = new Map((media.clips ?? []).map((m) => [m.scene_id, m]))
  const audioById = new Map((media.audios ?? []).map((m) => [m.scene_id, m]))
  return board.storyboard.scenes
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

/**
 * 未完成镜头汇总：只做告知，重试动作放在版本树那张卡片上。
 * 同一屏出现两个同名同行为的按钮会让人怀疑它们有区别，所以动作只有一处。
 */
function MissingBanner({ ep, missing }: { ep: Episode; missing: number[] }) {
  const media = activeNodeOfStage(ep, 'media')
  const clipErr = new Map((media?.clips ?? []).filter((m) => m.err).map((m) => [m.scene_id, m.err as string]))
  const audioErr = new Map((media?.audios ?? []).filter((m) => m.err).map((m) => [m.scene_id, m.err as string]))
  return (
    <div className="rounded-xl border border-seal-500/40 bg-seal-600/10 px-4 py-3 space-y-2">
      <span className="text-sm text-seal-500 font-medium">
        {missing.length} 个镜头未完成：{formatScenes(missing)}
      </span>
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
        到上方版本树选中「生产画面」那一版，点卡片上的「▶ 重跑本步」即可补齐这些镜头；
        已成功的画面与旁白直接复用，不会重复出图/合成。
      </p>
    </div>
  )
}

/**
 * 选中节点的产物详情，分上下两段：
 *   上段「上游产出」＝这个操作吃进去的东西（默认收起，需要对照时再展开）；
 *   下段「本步产出」＝这个操作产出的东西，只针对本步的操作按钮也在这里。
 * 这样「故事换了一版之后分镜为什么变了」不用来回滚动去找。
 */
function NodeDetail({
  ep,
  node,
  busy,
  pending,
  onAction,
}: {
  ep: Episode
  node: VersionNode
  busy: boolean
  pending: boolean
  onAction: (a: ActionName, extra?: ActExtra) => void
}) {
  const parent = node.parent_id ? (ep.nodes.find((n) => n.id === node.parent_id) ?? null) : null
  const disabled = busy || pending
  return (
    <div className="space-y-5">
      {parent && (
        <Collapsible
          summary={`上游产出 · ${STAGE_LABEL[parent.stage]} v${parent.attempt + 1}（本步的输入，展开可对照）`}
          badge={<StatusBadge status={parent.status} />}
        >
          <NodeBody ep={ep} node={parent} />
        </Collapsible>
      )}
      <Card
        title={
          <span className="flex items-center gap-3">
            <span>
              本步产出 · {STAGE_LABEL[node.stage]} v{node.attempt + 1}
            </span>
            <StatusBadge status={node.status} />
            {node.id === ep.active_node_id && (
              <span className="text-[10px] rounded bg-gold-500/20 px-1.5 py-0.5 text-gold-500">当前使用</span>
            )}
          </span>
        }
        extra={<span className="text-xs text-paper-300/30 font-body">{node.id}</span>}
      >
        {node.note && (
          <p className="mb-4 rounded-lg border border-gold-500/30 bg-gold-500/5 px-3.5 py-2 text-xs leading-5 text-gold-500/80">
            本版要求：{node.note}
          </p>
        )}
        <NodeBody
          ep={ep}
          node={node}
          disabled={disabled}
          onRetryScene={(sceneId) => {
            if (node.stage === 'media' && node.parent_id) {
              onAction('produce', { from: node.parent_id, scenes: [sceneId] })
            }
          }}
          onExport={(ratio) => onAction('export', { ratio, from: node.id })}
        />
      </Card>
    </div>
  )
}

/** NodeBody 按阶段渲染一个节点的产物；画面与成片阶段额外带上本步的操作按钮。 */
function NodeBody({
  ep,
  node,
  disabled = false,
  onRetryScene,
  onExport,
}: {
  ep: Episode
  node: VersionNode
  disabled?: boolean
  onRetryScene?: (id: number) => void
  onExport?: (ratio: string) => void
}) {
  switch (node.stage) {
    case 'story':
      return <StoryBody story={node.story} error={node.error} />
    case 'storyboard':
      return <StoryboardBody board={node} />
    case 'media':
      return (
        <MediaBody
          ep={ep}
          board={ep.nodes.find((n) => n.id === node.parent_id)}
          media={node}
          disabled={disabled}
          onRetryScene={onRetryScene}
        />
      )
    case 'final':
      return <FinalBody ep={ep} node={node} disabled={disabled} onExport={onExport} />
    default:
      return null
  }
}

/** 故事正文（只读）。 */
function StoryBody({ story, error }: { story?: StoryCandidate; error?: string }) {
  if (!story) {
    return (
      <>
        {error && <ErrorBox>{error}</ErrorBox>}
        <Empty text="本版本尚未产出故事。" />
      </>
    )
  }
  return (
    <>
      <p className="mb-3 text-sm text-paper-300/60 leading-relaxed">
        {story.dynasty} · {story.source} — {story.summary}
      </p>
      <div className="rounded-lg bg-ink-950/50 border border-ink-800 p-5 text-[15px] leading-8 text-paper-300/90 whitespace-pre-wrap font-display">
        {story.content}
      </div>
    </>
  )
}

/** 分镜正文（只读）：逐镜列出画面描述与旁白，供下游对照。 */
function StoryboardBody({ board }: { board: VersionNode }) {
  const sb: Storyboard | undefined = board.storyboard
  if (!sb) {
    return (
      <>
        {board.error && <ErrorBox>{board.error}</ErrorBox>}
        <Empty text="本版本尚未产出分镜。" />
      </>
    )
  }
  return (
    <ol className="space-y-3">
      {sb.scenes.map((sc) => (
        <li key={sc.id} className="rounded-lg border border-ink-800 bg-ink-950/40 p-3.5">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="font-display text-gold-500">镜 {String(sc.id).padStart(2, '0')}</span>
            <span className="text-xs rounded bg-ink-800 px-2 py-0.5 text-paper-300/60">{sc.duration}s</span>
            {sc.camera && <span className="text-xs text-paper-300/50">运镜：{sc.camera}</span>}
          </div>
          <div className="text-xs text-paper-300/40 mb-1">画面（已做朝代视觉锚定）</div>
          <p className="text-sm leading-7 text-paper-300/85">{sc.visual_prompt}</p>
          <div className="text-xs text-paper-300/40 mt-2.5 mb-1">旁白</div>
          <p className="text-sm leading-7 text-paper-100/90">{sc.narration}</p>
        </li>
      ))}
    </ol>
  )
}

/**
 * 画面正文：每镜同时给出「旁白文字」与「画面/旁白产物」，便于逐镜对照。
 * 旁白文字只存在上游分镜节点里（画面节点只登记媒体文件），所以必须回上游取。
 */
function MediaBody({
  ep,
  board,
  media,
  disabled,
  onRetryScene,
}: {
  ep: Episode
  board?: VersionNode
  media: VersionNode
  disabled: boolean
  onRetryScene?: (id: number) => void
}) {
  const sb: Storyboard | undefined = board?.storyboard
  if (!sb) {
    return (
      <>
        {media.error && <ErrorBox>{media.error}</ErrorBox>}
        <Empty text="本版本尚未产出画面与旁白。" />
      </>
    )
  }
  const clipById = new Map<number, MediaResult>((media.clips ?? []).map((m) => [m.scene_id, m]))
  const audioById = new Map<number, MediaResult>((media.audios ?? []).map((m) => [m.scene_id, m]))
  return (
    <div className="space-y-3">
      {media.error && <ErrorBox>{media.error}</ErrorBox>}
      <ol className="space-y-3">
        {sb.scenes.map((sc) => (
          <SceneCard
            key={sc.id}
            ep={ep}
            scene={sc}
            clip={clipById.get(sc.id)}
            audio={audioById.get(sc.id)}
            disabled={disabled}
            onRetry={onRetryScene && (() => onRetryScene(sc.id))}
          />
        ))}
      </ol>
    </div>
  )
}

/** 单镜卡片：左列是旁白原文与画面描述（对照用），右列是视频与旁白产物。 */
function SceneCard({
  ep,
  scene: sc,
  clip,
  audio,
  disabled,
  onRetry,
}: {
  ep: Episode
  scene: Scene
  clip?: MediaResult
  audio?: MediaResult
  disabled: boolean
  onRetry?: () => void
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
        <div className="grid lg:grid-cols-[1fr_300px] gap-4">
          <div className="space-y-3 min-w-0">
            <div>
              <div className="text-xs text-paper-300/40 mb-1">旁白</div>
              <p className="text-sm leading-7 text-paper-100/90">{sc.narration}</p>
            </div>
            <div>
              <div className="text-xs text-paper-300/40 mb-1">画面（已做朝代视觉锚定）</div>
              <p className="text-sm leading-7 text-paper-300/85">{sc.visual_prompt}</p>
            </div>
            {incomplete && onRetry && (
              <div className="flex flex-wrap items-center gap-2.5">
                <Button variant="seal" className="px-3 py-1.5 text-xs" disabled={disabled} onClick={onRetry}>
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

  const hasStoryboard = ep.nodes.some((n) => n.stage === 'storyboard')
  const badge = (
    <span className="text-xs text-paper-300/40">
      {refs.length} 条参考 · {withImage} 张图
    </span>
  )

  return (
    <Collapsible
      summary="本集视觉参考（人物 / 场景一致性约束，跨分镜版本共享）"
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
                ? '本分镜为旧版数据，未产出视觉参考。重新执行一次「拆分分镜」即可获得人物/场景的文字约束（不产生图片费用）。'
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

/** 成片正文：播放各比例成片，并给出导出其他比例的入口（合成不调模型，无提示词）。 */
function FinalBody({
  ep,
  node,
  disabled,
  onExport,
}: {
  ep: Episode
  node: VersionNode
  disabled: boolean
  onExport?: (ratio: string) => void
}) {
  const outs = node.outputs ?? []
  const ratios = ['16:9', '9:16', '1:1', '3:4']
  return (
    <div className="space-y-4">
      {node.error && <ErrorBox>{node.error}</ErrorBox>}
      {onExport && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-paper-300/45">导出其他比例：</span>
          {ratios.map((r) => (
            <Button
              key={r}
              variant="outline"
              className="px-2.5 py-1 text-xs"
              disabled={disabled}
              onClick={() => onExport(r)}
            >
              {r}
            </Button>
          ))}
        </div>
      )}
      {outs.length === 0 ? (
        <Empty text="本版本尚无成片。" />
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
    </div>
  )
}
