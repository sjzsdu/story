import { useState, type MouseEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { Episode } from '../types'
import { Button, Card, Empty, ErrorBox, Field, Spinner, TextInput } from '../components/ui'
import { StatusBadge } from '../components/ui'
import PlanPanel from '../components/PlanPanel'

function episodeProgress(ep: Episode) {
  if (ep.state.steps.compose.status === 'done') {
    return { step: 'compose' as const, status: ep.state.steps.compose.status }
  }
  return { step: ep.state.current, status: ep.state.steps[ep.state.current].status }
}

export default function SeriesDetailPage() {
  const { seriesId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({
    queryKey: ['series', seriesId],
    queryFn: () => api.getSeries(seriesId),
  })
  const [creating, setCreating] = useState(false)

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

  return (
    <div className="space-y-6">
      <nav className="text-sm text-paper-300/45">
        <Link to="/" className="hover:text-gold-500">
          系列
        </Link>
        <span className="mx-2">/</span>
        <span className="text-paper-300/80">{s.name}</span>
      </nav>

      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-3xl tracking-wider flex items-center gap-3">
            {s.name}
            {s.dynasty && (
              <span className="text-sm rounded border border-seal-500/40 bg-seal-600/10 px-2 py-0.5 text-seal-500 font-body">
                {s.dynasty}
              </span>
            )}
          </h1>
          {s.description && <p className="mt-2 text-sm text-paper-300/60 max-w-2xl">{s.description}</p>}
        </div>
        <div className="flex gap-3">
          <Button
            variant="ghost"
            onClick={() => {
              if (window.confirm(`确定删除系列「${s.name}」及其全部集与产物？此操作不可撤销。`)) {
                deleteSeriesMut.mutate()
              }
            }}
            disabled={deleteSeriesMut.isPending}
          >
            {deleteSeriesMut.isPending ? '删除中…' : '删除系列'}
          </Button>
          <Button variant="seal" onClick={() => setCreating((v) => !v)}>
            {creating ? '收起' : '＋ 新建一集'}
          </Button>
        </div>
      </div>

      <div className="grid md:grid-cols-3 gap-4 text-sm">
        <InfoTile label="画面 / 分辨率" value={`${s.config.ratio} · ${s.config.resolution}`} />
        <InfoTile label="TTS 音色" value={s.config.tts_voice} />
        <InfoTile
          label="并发 / 重试"
          value={`${s.config.max_concurrency} 路 · ${s.config.max_retries} 次退避重试`}
        />
      </div>
      {s.config.tts_instruction && (
        <Card title="旁白风格指令">
          <p className="text-sm text-paper-300/70 leading-relaxed">{s.config.tts_instruction}</p>
        </Card>
      )}

      <PlanPanel seriesId={s.id} existingTitles={episodes.map((e) => e.title)} />

      {creating && <CreateEpisodeCard seriesId={s.id} onDone={() => setCreating(false)} />}

      <Card title={`集列表（${episodes.length}）`}>
        {episodes.length === 0 ? (
          <Empty text="该系列还没有集" />
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
                      <span className="text-xs text-paper-300/40 hidden sm:inline">{p.step}</span>
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
    </div>
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

function CreateEpisodeCard({ seriesId, onDone }: { seriesId: string; onDone: () => void }) {
  const [title, setTitle] = useState('')
  const [topic, setTopic] = useState('')
  const [err, setErr] = useState('')
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: () => api.createEpisode(seriesId, { title: title.trim(), topic: topic.trim() }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series', seriesId] })
      onDone()
    },
    onError: (e) => setErr((e as Error).message),
  })

  return (
    <Card title="新建一集">
      <form
        className="grid sm:grid-cols-2 gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          setErr('')
          mutation.mutate()
        }}
      >
        <Field label="本集标题（必填）">
          <TextInput value={title} onChange={(e) => setTitle(e.target.value)} placeholder='如：入秦' autoFocus />
        </Field>
        <Field label="主题 / 切入点（可空，AI 自由命题）">
          <TextInput value={topic} onChange={(e) => setTopic(e.target.value)} placeholder="如：苏秦张仪出山之前" />
        </Field>
        {err && (
          <div className="sm:col-span-2">
            <ErrorBox>{err}</ErrorBox>
          </div>
        )}
        <div className="sm:col-span-2 flex gap-3">
          <Button type="submit" variant="seal" disabled={mutation.isPending || !title.trim()}>
            {mutation.isPending && <Spinner />} 创建
          </Button>
          <Button type="button" variant="ghost" onClick={onDone}>
            取消
          </Button>
        </div>
      </form>
    </Card>
  )
}
