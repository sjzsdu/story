import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { Episode, PublishJob } from '../types'
import { Button, Card, Empty, ErrorBox, Spinner } from '../components/ui'

const PLATFORM_ICONS: Record<string, string> = {
  douyin: '🎵',
  kuaishou: '⚡',
  bilibili: '📺',
  xiaohongshu: '📕',
  tencent: '💬',
  baijiahao: '📰',
  weibo: '🔴',
  hupu: '🏀',
  youtube: '▶️',
  tiktok: '🎶',
}

const STATUS_LABEL: Record<string, string> = {
  pending: '待发布',
  uploading: '上传中',
  uploaded: '已上传',
  published: '已发布',
  failed: '失败',
  rejected: '审核拒绝',
  canceled: '已取消',
}

/** 成片节点下方的发布面板：平台选择 + 素材填写 + 已有任务列表。 */
export default function PublishPanel({ ep }: { ep: Episode }) {
  const queryClient = useQueryClient()
  const [expanded, setExpanded] = useState(false)
  const [errMsg, setErrMsg] = useState('')

  // 平台列表
  const { data: platforms = [] } = useQuery({
    queryKey: ['platforms'],
    queryFn: api.listPlatforms,
  })

  // 已有发布任务
  const { data: jobs = [], isLoading: loadingJobs } = useQuery({
    queryKey: ['publish-jobs', ep.id],
    queryFn: () => api.listPublishJobs(ep.id),
    enabled: expanded,
  })

  // 发布表单状态
  const [selectedPlatforms, setSelectedPlatforms] = useState<Set<string>>(new Set())
  const [title, setTitle] = useState(ep.title || '')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')

  // 发布 mutation
  const publishMut = useMutation({
    mutationFn: () =>
      api.publishEpisode(ep.id, {
        platforms: Array.from(selectedPlatforms),
        title: title.trim() || undefined,
        description: description.trim() || undefined,
        tags: tags.trim() ? tags.split(/[,，\s]+/).filter(Boolean) : undefined,
      }),
    onError: (e) => setErrMsg((e as Error).message),
    onSuccess: () => {
      setErrMsg('')
      setSelectedPlatforms(new Set())
      void queryClient.invalidateQueries({ queryKey: ['publish-jobs', ep.id] })
    },
  })

  // 取消 mutation
  const cancelMut = useMutation({
    mutationFn: (jobId: string) => api.cancelPublishJob(ep.id, jobId),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['publish-jobs', ep.id] }),
  })

  // 删除 mutation
  const deleteMut = useMutation({
    mutationFn: (jobId: string) => api.deletePublishJob(ep.id, jobId),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['publish-jobs', ep.id] }),
  })

  const togglePlatform = (key: string) => {
    setSelectedPlatforms((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const publishable = selectedPlatforms.size > 0

  return (
    <div className="space-y-3">
      {/* 折叠触发器 */}
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between px-4 py-2.5 rounded-lg border border-ink-700 bg-ink-900/50 hover:border-gold-500/40 transition-colors"
      >
        <span className="flex items-center gap-2 text-sm text-paper-300/80">
          <span>🚀</span>
          <span>发布到平台</span>
          {jobs.length > 0 && (
            <span className="text-[10px] rounded bg-gold-500/20 px-1.5 py-0.5 text-gold-500">
              {jobs.length} 个任务
            </span>
          )}
        </span>
        <span className="text-xs text-paper-300/40">{expanded ? '收起' : '展开'}</span>
      </button>

      {expanded && (
        <div className="space-y-4">
          {errMsg && <ErrorBox>{errMsg}</ErrorBox>}

          {/* 平台选择 */}
          <Card title="选择平台">
            {platforms.length === 0 ? (
              <Empty text="未检测到可用平台。请先运行 story platform check。" />
            ) : (
              <div className="flex flex-wrap gap-2">
                {platforms.map((p) => (
                  <button
                    key={p.key}
                    onClick={() => togglePlatform(p.key)}
                    className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors ${
                      selectedPlatforms.has(p.key)
                        ? 'bg-gold-500/20 text-gold-500 border border-gold-500/40'
                        : 'bg-ink-800/50 text-paper-300/60 border border-ink-700 hover:border-ink-600'
                    }`}
                  >
                    <span>{PLATFORM_ICONS[p.key] ?? '🔗'}</span>
                    <span>{p.name}</span>
                  </button>
                ))}
              </div>
            )}
          </Card>

          {/* 素材填写 */}
          {publishable && (
            <Card title="发布素材">
              <div className="space-y-3 text-sm">
                <div>
                  <label className="block text-paper-300/60 mb-1">标题</label>
                  <input
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    placeholder="视频标题"
                    className="w-full px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 placeholder:text-paper-300/30 font-body"
                  />
                </div>
                <div>
                  <label className="block text-paper-300/60 mb-1">描述</label>
                  <textarea
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    placeholder="视频描述（可选）"
                    rows={3}
                    className="w-full px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 placeholder:text-paper-300/30 font-body resize-none"
                  />
                </div>
                <div>
                  <label className="block text-paper-300/60 mb-1">标签（逗号分隔）</label>
                  <input
                    value={tags}
                    onChange={(e) => setTags(e.target.value)}
                    placeholder="历史, 鬼谷子, 战国"
                    className="w-full px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 placeholder:text-paper-300/30 font-body"
                  />
                </div>
                <Button
                  variant="seal"
                  className="px-4 py-1.5 text-xs"
                  disabled={publishMut.isPending}
                  onClick={() => publishMut.mutate()}
                >
                  {publishMut.isPending ? '发布中…' : `发布到 ${selectedPlatforms.size} 个平台`}
                </Button>
              </div>
            </Card>
          )}

          {/* 已有任务列表 */}
          <Card title="发布记录">
            {loadingJobs ? (
              <div className="flex justify-center py-6"><Spinner className="w-4 h-4" /></div>
            ) : jobs.length === 0 ? (
              <Empty text="暂无发布记录。" />
            ) : (
              <div className="space-y-2">
                {jobs.map((job) => (
                  <JobRow
                    key={job.id}
                    job={job}
                    onCancel={() => cancelMut.mutate(job.id)}
                    onDelete={() => {
                      if (window.confirm(`确定删除「${PLATFORM_ICONS[job.platform] ?? ''} ${job.platform}」的发布任务？`)) {
                        deleteMut.mutate(job.id)
                      }
                    }}
                  />
                ))}
              </div>
            )}
          </Card>
        </div>
      )}
    </div>
  )
}

function JobRow({
  job,
  onCancel,
  onDelete,
}: {
  job: PublishJob
  onCancel: () => void
  onDelete: () => void
}) {
  const isActive = job.status === 'pending' || job.status === 'uploading'
  return (
    <div className="flex items-center gap-3 rounded-lg border border-ink-800 bg-ink-950/40 px-3 py-2">
      <span className="text-lg">{PLATFORM_ICONS[job.platform] ?? '🔗'}</span>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-xs text-paper-300/70">{job.platform}</span>
          <span className={`text-[10px] px-1.5 py-0.5 rounded ${
            job.status === 'published' ? 'bg-emerald-500/20 text-emerald-400' :
            job.status === 'failed' ? 'bg-red-500/20 text-red-400' :
            job.status === 'rejected' ? 'bg-orange-500/20 text-orange-400' :
            'bg-ink-700 text-paper-300/60'
          }`}>
            {STATUS_LABEL[job.status] ?? job.status}
          </span>
        </div>
        {job.title && <div className="text-xs text-paper-300/50 truncate mt-0.5">{job.title}</div>}
        {job.error && <div className="text-[11px] text-red-400/70 mt-0.5 truncate">{job.error}</div>}
        {job.platform_url && (
          <a href={job.platform_url} target="_blank" rel="noopener" className="text-[11px] text-gold-500/70 hover:text-gold-500 mt-0.5 inline-block">
            查看发布 ↗
          </a>
        )}
      </div>
      <div className="flex items-center gap-1 shrink-0">
        {isActive && (
          <Button variant="ghost" className="px-2 py-1 text-xs text-paper-300/50" onClick={onCancel}>
            取消
          </Button>
        )}
        {!isActive && (
          <Button variant="ghost" className="px-2 py-1 text-xs text-red-400/50 hover:text-red-400" onClick={onDelete}>
            删除
          </Button>
        )}
      </div>
    </div>
  )
}
