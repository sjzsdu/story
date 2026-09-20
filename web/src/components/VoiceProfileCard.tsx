import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { Button, ErrorBox, Spinner } from './ui'

/**
 * VoiceProfileCard 系列详情页「声音」卡片（§16）。
 * §16 起声音顶层实体化：series.voice_id 创建后锁定不可改，本卡片改为只读展示 + 试听；
 * 编辑入口移到独立的「声音」页（/voices）。
 */
export default function VoiceProfileCard({ seriesId }: { seriesId: string }) {
  const [previewPath, setPreviewPath] = useState<string | null>(null)

  // 拉取声音列表用于回查当前 series 引用的条目详情。
  const { data: voices, isLoading } = useQuery({
    queryKey: ['voices'],
    queryFn: api.listVoices,
    staleTime: Infinity,
  })

  const previewMut = useMutation({
    mutationFn: (voiceID: string) => api.previewVoice({ voice_id: voiceID }),
    onSuccess: (data) => setPreviewPath(data.path),
  })

  // 通过 series 详情查 voice_id 时不重复拉系列——上层传入 series 已有 voice_id；
  // 这里直接靠声音列表里查匹配条目（声音数量有限，前端过滤即可）。
  // 上层 SeriesDetailPage 用此组件时已传入 seriesId，靠 voices query + s.voice_id 反查。
  void seriesId // 暂时保留 prop 签名兼容上层调用，未来可移除

  // 找当前 series 引用的声音条目：先按 query 拉 series 拿 voice_id，再在 voices 里反查。
  const { data: series } = useQuery({
    queryKey: ['series', seriesId],
    queryFn: () => api.getSeries(seriesId),
    staleTime: 30_000,
  })
  const current = series?.series
  const voiceID = current?.voice_id || ''
  const voice = voices?.find((v) => v.id === voiceID)

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Spinner className="w-5 h-5" />
      </div>
    )
  }

  if (!voiceID) {
    return (
      <div className="space-y-3">
        <p className="text-xs text-paper-300/50">
          该系列尚未关联声音条目（旧数据迁移前）。重启服务后 Bootstrap 会自动平迁 voice_id。
        </p>
      </div>
    )
  }

  if (!voice) {
    return (
      <div className="space-y-3">
        <ErrorBox>
          声音条目 {voiceID} 不存在（可能已被删除）。请到「声音」页新建同名条目或重建系列。
        </ErrorBox>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-ink-700 bg-ink-950/40 p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className="flex items-center gap-2">
              <span className="font-display text-gold-500">{voice.name}</span>
              {voice.is_builtin && (
                <span className="text-[11px] rounded-full px-2 py-0.5 border text-emerald-300/70 border-emerald-400/30 bg-emerald-400/10">
                  内置
                </span>
              )}
              <span className="text-[11px] rounded-full px-2 py-0.5 border border-ink-600 text-paper-300/50">
                {voice.id}
              </span>
            </div>
            {voice.style_note && (
              <p className="mt-1 text-xs text-paper-300/55 leading-relaxed">{voice.style_note}</p>
            )}
          </div>
          <Button
            variant="outline"
            disabled={previewMut.isPending}
            onClick={() => previewMut.mutate(voice.id)}
          >
            {previewMut.isPending ? <Spinner className="w-3.5 h-3.5" /> : null}
            {previewMut.isPending ? '合成中…' : '试听'}
          </Button>
        </div>

        <dl className="mt-3 text-xs text-paper-300/50 space-y-1">
          <div className="flex gap-2">
            <dt className="w-14 shrink-0 text-paper-300/35">BL音色</dt>
            <dd>{voice.voice}</dd>
          </div>
          {(voice.rate ?? 0) > 0 && (
            <div className="flex gap-2">
              <dt className="w-14 shrink-0 text-paper-300/35">语速</dt>
              <dd>{(voice.rate ?? 0).toFixed(2)}</dd>
            </div>
          )}
          {(voice.pitch ?? 0) > 0 && (voice.pitch ?? 0) !== 1 && (
            <div className="flex gap-2">
              <dt className="w-14 shrink-0 text-paper-300/35">音高</dt>
              <dd>{(voice.pitch ?? 0).toFixed(2)}</dd>
            </div>
          )}
          {voice.instruction && (
            <div className="flex gap-2">
              <dt className="w-14 shrink-0 text-paper-300/35">指令</dt>
              <dd className="leading-relaxed">{voice.instruction}</dd>
            </div>
          )}
        </dl>
      </div>

      <p className="text-xs text-paper-300/35">
        声音创建后锁定，无法在此卡片修改。如需调整，请到「声音」页编辑条目本身（修改后影响所有引用它的系列）。
      </p>

      {previewMut.isError && <ErrorBox>{(previewMut.error as Error).message}</ErrorBox>}

      {previewPath && (
        <div className="rounded-lg border border-ink-700 bg-ink-950/40 p-3">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-paper-300/50">试听样音</span>
            <button
              className="text-xs text-paper-300/30 hover:text-paper-300/60"
              onClick={() => setPreviewPath(null)}
            >
              关闭
            </button>
          </div>
          <audio
            controls
            autoPlay
            className="w-full h-9"
            src={`/api/voices/preview?path=${encodeURIComponent(previewPath)}`}
          />
        </div>
      )}
    </div>
  )
}
