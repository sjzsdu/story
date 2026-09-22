import { useState, type MouseEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { CreativeStyle, Series } from '../types'
import { Button, Card, Empty, ErrorBox, Field, Modal, Select, Spinner, TextInput } from '../components/ui'
import CreativeFields, { knobMap } from '../components/CreativeFields'

export default function SeriesListPage() {
  const { data: series, isLoading, error } = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
  const [open, setOpen] = useState(false)

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="font-display text-2xl tracking-wider">内容系列</h1>
          <p className="mt-1 text-sm text-paper-300/50">系列 → 多集，每集一条可中断、可续跑的生产流水线</p>
        </div>
        <Button variant="seal" onClick={() => setOpen(true)}>
          ＋ 新建系列
        </Button>
      </div>

      {error && <ErrorBox>{(error as Error).message}</ErrorBox>}

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner className="w-6 h-6" />
        </div>
      ) : !series || series.length === 0 ? (
        <Card>
          <Empty text="还没有系列。点击「新建系列」开始，例如：鬼谷子 / 史记名将 / 世说新语" />
        </Card>
      ) : (
        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {series.map((s) => (
            <SeriesCard key={s.id} series={s} />
          ))}
        </div>
      )}

      <CreateSeriesModal open={open} onClose={() => setOpen(false)} />
    </div>
  )
}

/** 新建系列弹窗：画面模式与声音创建后锁定，不可修改。 */
function CreateSeriesModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [dynasty, setDynasty] = useState('')
  const [ratio, setRatio] = useState('9:16')
  const [resolution, setResolution] = useState('1080P')
  const [visualMode, setVisualMode] = useState<'comic' | 'video'>('comic')
  // §16：声音条目 ID 必选（默认内置 longtian 龙天）；可在「声音」页管理条目。
  const [voiceID, setVoiceID] = useState('longtian')
  // 创作控制参数：一个 state 装下预设 + 逐项值（预设 key 存在 values.preset 里）。
  const [creative, setCreative] = useState<CreativeStyle>({})
  // 系列级 Provider 覆盖（空＝用系统默认）
  const [textProvider, setTextProvider] = useState('')
  const [ttsProvider, setTtsProvider] = useState('')
  const [imageProvider, setImageProvider] = useState('')
  const [videoProvider, setVideoProvider] = useState('')
  const [err, setErr] = useState('')
  const [step, setStep] = useState(1)

  // 分步填写：名称是唯一必填项，因此只有它决定能否往下走。
  const canAdvance = name.trim().length > 0
  const closeModal = () => {
    setStep(1)
    setErr('')
    onClose()
  }

  // 拉取声音列表供下拉；staleTime=Infinity 避免每次开关都重新拉。
  const { data: voices } = useQuery({
    queryKey: ['voices'],
    queryFn: api.listVoices,
    staleTime: Infinity,
  })

  // 创作参数注册表（含预设）：同样缓存不失效。
  const { data: catalog } = useQuery({
    queryKey: ['creative-catalog'],
    queryFn: api.getCreativeCatalog,
    staleTime: Infinity,
  })

  const mutation = useMutation({
    mutationFn: () => {
      // 组装创作字段：套用预设时必须提交全部参数（含空串），否则「清空某项」无法覆盖预设值；
      // 未套预设时只提交非空项——全空则 creative/preset 都不出现，与历史请求逐字一致。
      const payload: { preset?: string; creative?: Record<string, string> } = {}
      if (creative.preset) {
        payload.preset = creative.preset
        if (catalog) payload.creative = knobMap(creative, catalog)
      } else {
        const tuned: Record<string, string> = {}
        for (const [k, v] of Object.entries(creative)) {
          if (k !== 'preset' && v) tuned[k] = v
        }
        if (Object.keys(tuned).length > 0) payload.creative = tuned
      }
      return api.createSeries({
        name: name.trim(),
        dynasty: dynasty.trim(),
        ratio,
        resolution,
        visual_mode: visualMode,
        voice_id: voiceID,
        text_provider: textProvider || undefined,
        tts_provider: ttsProvider || undefined,
        image_provider: imageProvider || undefined,
        video_provider: videoProvider || undefined,
        ...payload,
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['series'] })
      setName('')
      setDynasty('')
      setRatio('9:16')
      setResolution('1080P')
      setVisualMode('comic')
      setVoiceID('longtian')
      setCreative({})
      setTextProvider('')
      setTtsProvider('')
      setImageProvider('')
      setVideoProvider('')
      setErr('')
      setStep(1)
      onClose()
    },
    onError: (e) => setErr((e as Error).message),
  })

  return (
    <Modal open={open} onClose={closeModal} title="新建系列" wide>
      <form
        className="grid sm:grid-cols-2 gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          setErr('')
          // 回车＝下一步（最后一步才真正提交）。
          if (step < STEPS.length) {
            if (canAdvance) setStep(step + 1)
            return
          }
          mutation.mutate()
        }}
      >
        <div className="sm:col-span-2 flex flex-wrap items-center gap-2">
          {STEPS.map((s) => {
            const active = step === s.key
            return (
              <button
                key={s.key}
                type="button"
                disabled={s.key > step && !canAdvance}
                onClick={() => setStep(s.key)}
                className={`flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs transition-colors disabled:cursor-not-allowed ${
                  active
                    ? 'border-seal-600 bg-seal-600/15 text-paper-100'
                    : step > s.key
                      ? 'border-ink-700 text-paper-300/70 hover:text-paper-100'
                      : 'border-ink-800 text-paper-300/35 hover:text-paper-300/60 disabled:hover:text-paper-300/35'
                }`}
              >
                <span
                  className={`grid h-4 w-4 place-items-center rounded-full font-display text-[10px] ${
                    active ? 'bg-seal-600 text-paper-100' : 'bg-ink-800 text-paper-300/60'
                  }`}
                >
                  {s.key}
                </span>
                {s.label}
              </button>
            )
          })}
        </div>

        {step === 1 && (
          <>
            <Field label="系列名称（必填）">
              <TextInput
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="如：鬼谷子"
                autoFocus
              />
            </Field>
            <Field label="朝代锚定">
              <TextInput value={dynasty} onChange={(e) => setDynasty(e.target.value)} placeholder="如：战国" />
            </Field>
          </>
        )}

        {step === 2 && (
          <>
            <Field label="画面比例">
              <Select value={ratio} onChange={(e) => setRatio(e.target.value)}>
                <option>9:16</option>
                <option>16:9</option>
                <option>1:1</option>
                <option>3:4</option>
              </Select>
            </Field>
            <Field label="分辨率（档位为长边）">
              <Select value={resolution} onChange={(e) => setResolution(e.target.value)}>
                <option>1080P</option>
                <option>720P</option>
              </Select>
            </Field>
            <Field label="画面模式（创建后不可更改）">
              <Select
                value={visualMode}
                onChange={(e) => setVisualMode(e.target.value === 'video' ? 'video' : 'comic')}
              >
                <option value="comic">小人书插画（默认 · 省钱）</option>
                <option value="video">AI 生成视频（动态 · 较贵）</option>
              </Select>
            </Field>
            <Field label="声音（创建后不可更改）">
              <Select value={voiceID} onChange={(e) => setVoiceID(e.target.value)}>
                {voices?.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.name} {v.is_builtin ? '（内置）' : ''} · {v.voice}
                  </option>
                ))}
              </Select>
            </Field>
          </>
        )}

        {step === 3 && (
          <>
            <div className="sm:col-span-2">
              <CreativeFields catalog={catalog} values={creative} onChange={setCreative} />
            </div>
            <p className="sm:col-span-2 text-xs text-paper-300/40">
              将创建：{name.trim()}
              {dynasty.trim() && ` · ${dynasty.trim()}`} · {ratio} · {resolution} ·{' '}
              {visualMode === 'comic' ? '小人书插画' : 'AI 生成视频'} ·{' '}
              {voices?.find((v) => v.id === voiceID)?.name ?? voiceID}
            </p>
          </>
        )}

        {step === 4 && (
          <>
            <div className="sm:col-span-2">
              <p className="text-xs text-paper-300/50 mb-3">
                系列级 Provider 覆盖 — 空值使用系统默认配置，选定后该系列所有集均使用指定 Provider。
              </p>
            </div>
            <Field label="文本生成">
              <Select value={textProvider} onChange={(e) => setTextProvider(e.target.value)}>
                <option value="">系统默认</option>
                <option value="bailian">百炼 (bl)</option>
                <option value="deepseek">DeepSeek</option>
              </Select>
            </Field>
            <Field label="语音合成">
              <Select value={ttsProvider} onChange={(e) => setTtsProvider(e.target.value)}>
                <option value="">系统默认</option>
                <option value="bailian">百炼 (CosyVoice)</option>
                <option value="minimax">MiniMax</option>
              </Select>
            </Field>
            <Field label="图片生成">
              <Select value={imageProvider} onChange={(e) => setImageProvider(e.target.value)}>
                <option value="">系统默认</option>
                <option value="bailian">百炼 (通义万相)</option>
                <option value="zhipu">智谱 (CogView)</option>
              </Select>
            </Field>
            <Field label="视频生成">
              <Select value={videoProvider} onChange={(e) => setVideoProvider(e.target.value)}>
                <option value="">系统默认</option>
                <option value="bailian">百炼 (Wanx Video)</option>
                <option value="kling">可灵 (Kling)</option>
              </Select>
            </Field>
          </>
        )}

        {err && (
          <div className="sm:col-span-2">
            <ErrorBox>{err}</ErrorBox>
          </div>
        )}

        <div className="sm:col-span-2 flex items-center gap-3">
          {step > 1 && (
            <Button type="button" variant="ghost" onClick={() => setStep(step - 1)}>
              上一步
            </Button>
          )}
          {step < STEPS.length ? (
            <Button type="submit" variant="seal" disabled={!canAdvance}>
              下一步
            </Button>
          ) : (
            <Button type="submit" variant="seal" disabled={mutation.isPending || !canAdvance}>
              {mutation.isPending && <Spinner />} 创建
            </Button>
          )}
          <Button type="button" variant="ghost" onClick={closeModal}>
            取消
          </Button>
          <span className="ml-auto text-xs text-paper-300/35">
            第 {step} / {STEPS.length} 步
          </span>
        </div>
      </form>
    </Modal>
  )
}

const STEPS = [
  { key: 1, label: '基本信息' },
  { key: 2, label: '规格与声音' },
  { key: 3, label: '创作预设' },
  { key: 4, label: 'Provider' },
]

function SeriesCard({ series: s }: { series: Series }) {
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: () => api.deleteSeries(s.id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['series'] }),
  })
  const onDelete = (e: MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (window.confirm(`确定删除系列「${s.name}」及其全部集与产物？此操作不可撤销。`)) {
      mutation.mutate()
    }
  }
  return (
    <Link
      to={`/series/${s.id}`}
      className="block rounded-xl border border-ink-800 bg-ink-900/70 p-5 hover:border-gold-500/50 hover:bg-ink-900 transition-colors group"
    >
      <div className="flex items-start justify-between gap-3">
        <h2 className="font-display text-xl tracking-wider group-hover:text-gold-500 transition-colors">
          {s.name}
        </h2>
        <div className="flex items-center gap-2">
          {s.dynasty && (
            <span className="shrink-0 rounded border border-seal-500/40 bg-seal-600/10 px-2 py-0.5 text-xs text-seal-500">
              {s.dynasty}
            </span>
          )}
          <button
            type="button"
            onClick={onDelete}
            disabled={mutation.isPending}
            title="删除系列"
            className="shrink-0 rounded px-1.5 py-0.5 text-xs text-seal-500/70 opacity-60 hover:opacity-100 hover:bg-seal-600/20"
          >
            {mutation.isPending ? '…' : '✕'}
          </button>
        </div>
      </div>
      <dl className="mt-4 space-y-1.5 text-xs text-paper-300/60">
        <div className="flex gap-2">
          <dt className="w-14 shrink-0 text-paper-300/35">画面</dt>
          <dd>
            {s.config.ratio} · {s.config.resolution} · {s.config.visual_mode === 'video' ? 'AI视频' : '小人书'}
          </dd>
        </div>
        <div className="flex gap-2">
          <dt className="w-14 shrink-0 text-paper-300/35">声音</dt>
          <dd className="truncate">{s.voice_id || s.config.tts_voice || '默认'}</dd>
        </div>
        <div className="flex gap-2">
          <dt className="w-14 shrink-0 text-paper-300/35">策略</dt>
          <dd>
            并发 {s.config.max_concurrency} · 重试 {s.config.max_retries}
          </dd>
        </div>
      </dl>
    </Link>
  )
}
