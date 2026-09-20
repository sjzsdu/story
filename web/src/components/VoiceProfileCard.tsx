import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { SeriesConfig } from '../types'
import { Button, Spinner } from './ui'

export default function VoiceProfileCard({
  seriesId,
  config,
}: {
  seriesId: string
  config: SeriesConfig
}) {
  const qc = useQueryClient()
  const [previewPath, setPreviewPath] = useState<string | null>(null)
  const [customMode, setCustomMode] = useState(!config.voice_profile)

  const { data: voices } = useQuery({
    queryKey: ['voices'],
    queryFn: api.listVoices,
    staleTime: Infinity,
  })

  const updateMut = useMutation({
    mutationFn: (body: Parameters<typeof api.updateVoiceProfile>[1]) =>
      api.updateVoiceProfile(seriesId, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['series', seriesId] })
      qc.invalidateQueries({ queryKey: ['seriesDetail', seriesId] })
    },
  })

  const previewMut = useMutation({
    mutationFn: (body: Parameters<typeof api.previewVoice>[0]) => api.previewVoice(body),
    onSuccess: (data) => setPreviewPath(data.path),
  })

  const currentProfile = config.voice_profile || ''

  return (
    <div className="space-y-5">
      {/* 模式切换 */}
      <div className="flex gap-2">
        <button
          className={`px-3 py-1.5 rounded-lg text-sm transition ${!customMode ? 'bg-gold-500/20 text-gold-400' : 'bg-ink-800 text-paper-300/50 hover:text-paper-300/80'}`}
          onClick={() => setCustomMode(false)}
        >
          预设语音
        </button>
        <button
          className={`px-3 py-1.5 rounded-lg text-sm transition ${customMode ? 'bg-gold-500/20 text-gold-400' : 'bg-ink-800 text-paper-300/50 hover:text-paper-300/80'}`}
          onClick={() => setCustomMode(true)}
        >
          自定义
        </button>
      </div>

      {!customMode && voices ? (
        <div className="space-y-3">
          <p className="text-xs text-paper-300/40">
            使用百炼系统声音模拟讲述风格，非真人声音克隆。点击「试听」会调用一次语音合成（按次计费）。
          </p>
          <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {voices.map((v) => {
              const active = v.key === currentProfile
              return (
                <div
                  key={v.key}
                  className={`rounded-xl border p-4 cursor-pointer transition ${
                    active
                      ? 'border-gold-500/60 bg-gold-500/5'
                      : 'border-ink-700 bg-ink-950/40 hover:border-ink-600'
                  }`}
                  onClick={() => {
                    updateMut.mutate({ profile: v.key })
                  }}
                >
                  <div className="flex items-center justify-between mb-1">
                    <span className="font-display text-gold-500 text-sm">{v.name}</span>
                    {active && <span className="text-xs text-gold-400">当前</span>}
                  </div>
                  <p className="text-xs text-paper-300/50 leading-relaxed">{v.style_note}</p>
                  <div className="mt-2 flex items-center gap-3 text-xs text-paper-300/35">
                    <span>{v.voice}</span>
                    {v.rate ? <span>语速 {v.rate}</span> : null}
                    {v.pitch && v.pitch !== 1 ? <span>音高 {v.pitch}</span> : null}
                  </div>
                  <button
                    className="mt-2 text-xs text-sky-400/70 hover:text-sky-400 disabled:opacity-30"
                    disabled={previewMut.isPending}
                    onClick={(e) => {
                      e.stopPropagation()
                      previewMut.mutate({ profile: v.key })
                    }}
                  >
                    {previewMut.isPending ? '合成中…' : '试听'}
                  </button>
                </div>
              )
            })}
          </div>
        </div>
      ) : customMode ? (
        <div className="space-y-4">
          <CustomVoiceEditor
            seriesId={seriesId}
            config={config}
            previewPending={previewMut.isPending}
            onPreviewReq={previewMut.mutate}
          />
        </div>
      ) : (
        <Spinner />
      )}

      {updateMut.isPending && <p className="text-xs text-paper-300/40">保存中…</p>}
      {updateMut.isError && <p className="text-xs text-seal-500">保存失败</p>}

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
          <audio controls autoPlay className="w-full h-9" src={`/api/voices/preview?path=${encodeURIComponent(previewPath)}`} />
        </div>
      )}
    </div>
  )
}

function CustomVoiceEditor({
  seriesId,
  config,
  previewPending,
  onPreviewReq,
}: {
  seriesId: string
  config: SeriesConfig
  previewPending: boolean
  onPreviewReq: (body: Parameters<typeof api.previewVoice>[0]) => void
}) {
  const qc = useQueryClient()
  const [voice, setVoice] = useState(config.tts_voice || 'longtian_v3')
  const [rate, setRate] = useState(config.tts_rate || 1.0)
  const [pitch, setPitch] = useState(config.tts_pitch || 1.0)
  const [instruction, setInstruction] = useState(config.tts_instruction || '')

  const saveMut = useMutation({
    mutationFn: () =>
      api.updateVoiceProfile(seriesId, { voice, rate, pitch, instruction }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['seriesDetail', seriesId] })
    },
  })

  return (
    <div className="space-y-4">
      <div>
        <label className="text-xs text-paper-300/40 block mb-1">声音 ID</label>
        <input
          className="w-full rounded-lg border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-paper-100"
          value={voice}
          onChange={(e) => setVoice(e.target.value)}
          placeholder="如 longtian_v3"
        />
        <p className="text-xs text-paper-300/30 mt-1">
          可选声音见 bl speech synthesize --list-voices，如 longtian_v3（磁性理智男）、longze_v3（温暖元气男）、longfei_v3（热血磁性男）等
        </p>
      </div>
      <div className="grid sm:grid-cols-2 gap-4">
        <div>
          <label className="text-xs text-paper-300/40 block mb-1">语速（0.5-2.0，默认 1.0）</label>
          <input
            type="range"
            min="0.5"
            max="2"
            step="0.05"
            value={rate}
            onChange={(e) => setRate(parseFloat(e.target.value))}
            className="w-full"
          />
          <span className="text-xs text-paper-300/50">{rate.toFixed(2)}</span>
        </div>
        <div>
          <label className="text-xs text-paper-300/40 block mb-1">音高（0.5-2.0，默认 1.0）</label>
          <input
            type="range"
            min="0.5"
            max="2"
            step="0.05"
            value={pitch}
            onChange={(e) => setPitch(parseFloat(e.target.value))}
            className="w-full"
          />
          <span className="text-xs text-paper-300/50">{pitch.toFixed(2)}</span>
        </div>
      </div>
      <div>
        <label className="text-xs text-paper-300/40 block mb-1">风格指令（可选，部分音色不支持）</label>
        <input
          className="w-full rounded-lg border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-paper-100"
          value={instruction}
          onChange={(e) => setInstruction(e.target.value)}
          placeholder="如：沉稳、有书卷气、节奏从容"
        />
      </div>
      <div className="flex gap-3">
        <Button
          onClick={() => saveMut.mutate()}
          disabled={saveMut.isPending}
        >
          {saveMut.isPending ? '保存中…' : '保存自定义'}
        </Button>
        <Button
          variant="ghost"
          onClick={() => onPreviewReq({ voice, rate, pitch, instruction })}
          disabled={previewPending}
        >
          {previewPending ? '合成中…' : '试听'}
        </Button>
        {saveMut.isSuccess && <span className="text-xs text-emerald-400/60 self-center">已保存</span>}
      </div>
    </div>
  )
}
