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
  const [err, setErr] = useState('')

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
      setErr('')
      onClose()
    },
    onError: (e) => setErr((e as Error).message),
  })

  return (
    <Modal open={open} onClose={onClose} title="新建系列" wide>
      <form
        className="grid sm:grid-cols-2 gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          setErr('')
          mutation.mutate()
        }}
      >
        <Field label="系列名称（必填）">
          <TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder="如：鬼谷子" autoFocus />
        </Field>
        <Field label="朝代锚定">
          <TextInput value={dynasty} onChange={(e) => setDynasty(e.target.value)} placeholder="如：战国" />
        </Field>
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
          <Select value={visualMode} onChange={(e) => setVisualMode(e.target.value === 'video' ? 'video' : 'comic')}>
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
        <div className="sm:col-span-2">
          <CreativeFields catalog={catalog} values={creative} onChange={setCreative} />
        </div>
        {err && (
          <div className="sm:col-span-2">
            <ErrorBox>{err}</ErrorBox>
          </div>
        )}
        <div className="sm:col-span-2 flex gap-3">
          <Button type="submit" variant="seal" disabled={mutation.isPending || !name.trim()}>
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
        {(s.config.target_platforms?.length ?? 0) > 0 && (
          <div className="flex gap-2">
            <dt className="w-14 shrink-0 text-paper-300/35">平台</dt>
            <dd>{s.config.target_platforms!.join(' / ')}</dd>
          </div>
        )}
      </dl>
    </Link>
  )
}
