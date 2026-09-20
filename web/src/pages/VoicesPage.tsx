import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { Voice } from '../types'
import { Button, Card, Empty, ErrorBox, Field, Modal, Select, Spinner, TextInput } from '../components/ui'

export default function VoicesPage() {
  const { data: voices, isLoading, error } = useQuery({ queryKey: ['voices'], queryFn: api.listVoices })
  const [showCreate, setShowCreate] = useState(false)
  const [editing, setEditing] = useState<Voice | null>(null)

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="font-display text-2xl tracking-wider">声音库</h1>
          <p className="mt-1 text-sm text-paper-300/50">
            与系列同级的顶层实体。新建系列时选定一个声音，之后锁定不可改；编辑条目本身会同步影响所有引用它的系列
          </p>
        </div>
        <Button variant="seal" onClick={() => setShowCreate(true)}>
          ＋ 新建声音
        </Button>
      </div>

      {error && <ErrorBox>{(error as Error).message}</ErrorBox>}

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner className="w-6 h-6" />
        </div>
      ) : !voices || voices.length === 0 ? (
        <Card>
          <Empty text="还没有声音条目（内置条目启动时会自动 seed，如缺失请重启服务）" />
        </Card>
      ) : (
        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {voices.map((v) => (
            <VoiceCard key={v.id} voice={v} onEdit={() => setEditing(v)} />
          ))}
        </div>
      )}

      <CreateVoiceModal open={showCreate} onClose={() => setShowCreate(false)} />
      {editing && <EditVoiceModal voice={editing} onClose={() => setEditing(null)} />}
    </div>
  )
}

function VoiceCard({ voice: v, onEdit }: { voice: Voice; onEdit: () => void }) {
  const qc = useQueryClient()
  const [previewPath, setPreviewPath] = useState<string | null>(null)
  const [err, setErr] = useState('')

  const previewMut = useMutation({
    mutationFn: () => api.previewVoice({ voice_id: v.id }),
    onSuccess: (data) => {
      setErr('')
      setPreviewPath(data.path)
    },
    onError: (e) => setErr((e as Error).message),
  })

  const deleteMut = useMutation({
    mutationFn: () => api.deleteVoice(v.id),
    onSuccess: () => {
      setErr('')
      void qc.invalidateQueries({ queryKey: ['voices'] })
    },
    onError: (e) => setErr((e as Error).message),
  })

  return (
    <div className="rounded-xl border border-ink-800 bg-ink-950/60 p-5 flex flex-col">
      <div className="flex items-start justify-between gap-2 mb-2">
        <div className="font-display text-gold-500">{v.name}</div>
        {v.is_builtin && (
          <span className="text-[11px] rounded-full px-2 py-0.5 border text-emerald-300/70 border-emerald-400/30 bg-emerald-400/10">
            内置
          </span>
        )}
      </div>

      <p className="text-xs text-paper-300/55 leading-relaxed mb-3">{v.style_note || '（无风格说明）'}</p>

      <dl className="text-xs text-paper-300/50 space-y-1 mb-3">
        <div className="flex gap-2">
          <dt className="w-14 shrink-0 text-paper-300/35">BL音色</dt>
          <dd className="truncate">{v.voice}</dd>
        </div>
        {(v.rate ?? 0) > 0 && (
          <div className="flex gap-2">
            <dt className="w-14 shrink-0 text-paper-300/35">语速</dt>
            <dd>{(v.rate ?? 0).toFixed(2)}</dd>
          </div>
        )}
        {(v.pitch ?? 0) > 0 && (v.pitch ?? 0) !== 1 && (
          <div className="flex gap-2">
            <dt className="w-14 shrink-0 text-paper-300/35">音高</dt>
            <dd>{(v.pitch ?? 0).toFixed(2)}</dd>
          </div>
        )}
        {v.instruction && (
          <div className="flex gap-2">
            <dt className="w-14 shrink-0 text-paper-300/35">指令</dt>
            <dd className="truncate" title={v.instruction}>{v.instruction}</dd>
          </div>
        )}
      </dl>

      <div className="mt-auto flex items-center gap-2 pt-2 border-t border-ink-800">
        <button
          className="text-xs text-sky-400/70 hover:text-sky-400 disabled:opacity-30"
          disabled={previewMut.isPending}
          onClick={() => previewMut.mutate()}
        >
          {previewMut.isPending ? '合成中…' : '试听'}
        </button>
        <button
          className="ml-auto text-xs text-paper-300/50 hover:text-paper-100"
          onClick={onEdit}
        >
          编辑
        </button>
        {!v.is_builtin && (
          <button
            className="text-xs text-seal-500/70 hover:text-seal-500 disabled:opacity-30"
            disabled={deleteMut.isPending}
            onClick={() => {
              if (window.confirm(`确定删除声音「${v.name}」？若被系列引用会被拒绝。`)) {
                deleteMut.mutate()
              }
            }}
          >
            {deleteMut.isPending ? '删除中…' : '删除'}
          </button>
        )}
      </div>

      {err && <div className="mt-2"><ErrorBox>{err}</ErrorBox></div>}

      {previewPath && (
        <div className="mt-3 rounded-lg border border-ink-700 bg-ink-950/40 p-2">
          <div className="flex items-center justify-between mb-1">
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

const DEFAULT_VOICES = [
  'longtian_v3',
  'longze_v3',
  'longcheng_v3',
  'longfei_v3',
  'longhao_v3',
  'longxiaoxia_v3',
]

function CreateVoiceModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [voice, setVoice] = useState('longtian_v3')
  const [instruction, setInstruction] = useState('')
  const [rate, setRate] = useState(0)
  const [pitch, setPitch] = useState(0)
  const [styleNote, setStyleNote] = useState('')
  const [err, setErr] = useState('')

  const mutation = useMutation({
    mutationFn: () =>
      api.createVoice({
        name: name.trim(),
        voice: voice.trim(),
        instruction: instruction.trim(),
        rate,
        pitch,
        style_note: styleNote.trim(),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['voices'] })
      setName('')
      setVoice('longtian_v3')
      setInstruction('')
      setRate(0)
      setPitch(0)
      setStyleNote('')
      setErr('')
      onClose()
    },
    onError: (e) => setErr((e as Error).message),
  })

  return (
    <Modal open={open} onClose={onClose} title="新建声音" wide>
      <form
        className="grid sm:grid-cols-2 gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          setErr('')
          mutation.mutate()
        }}
      >
        <Field label="名称（必填）">
          <TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder="如：磁性理智男" autoFocus />
        </Field>
        <Field label="百炼音色 ID（必填）">
          <Select value={voice} onChange={(e) => setVoice(e.target.value)}>
            {DEFAULT_VOICES.map((v) => (
              <option key={v} value={v}>{v}</option>
            ))}
            <option value="">{voice && !DEFAULT_VOICES.includes(voice) ? voice : '— 自定义 —'}</option>
          </Select>
          {!DEFAULT_VOICES.includes(voice) && (
            <TextInput
              className="mt-2"
              value={voice}
              onChange={(e) => setVoice(e.target.value)}
              placeholder="输入自定义百炼音色 ID"
            />
          )}
        </Field>
        <Field label="风格指令（可选，部分音色不支持）">
          <TextInput
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            placeholder="如：沉稳、有书卷气、节奏从容"
          />
        </Field>
        <Field label="风格说明（仅展示用）">
          <TextInput
            value={styleNote}
            onChange={(e) => setStyleNote(e.target.value)}
            placeholder="如：磁性理智男，语速沉稳"
          />
        </Field>
        <Field label="语速（0.5-2.0，0=默认）">
          <input
            type="range"
            min="0"
            max="2"
            step="0.05"
            value={rate}
            onChange={(e) => setRate(parseFloat(e.target.value))}
            className="w-full"
          />
          <span className="text-xs text-paper-300/50">{rate.toFixed(2)}</span>
        </Field>
        <Field label="音高（0.5-2.0，0=默认）">
          <input
            type="range"
            min="0"
            max="2"
            step="0.05"
            value={pitch}
            onChange={(e) => setPitch(parseFloat(e.target.value))}
            className="w-full"
          />
          <span className="text-xs text-paper-300/50">{pitch.toFixed(2)}</span>
        </Field>
        {err && (
          <div className="sm:col-span-2">
            <ErrorBox>{err}</ErrorBox>
          </div>
        )}
        <div className="sm:col-span-2 flex gap-3">
          <Button type="submit" variant="seal" disabled={mutation.isPending || !name.trim() || !voice.trim()}>
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

function EditVoiceModal({ voice: v, onClose }: { voice: Voice; onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState(v.name)
  const [voice, setVoice] = useState(v.voice)
  const [instruction, setInstruction] = useState(v.instruction || '')
  const [rate, setRate] = useState(v.rate || 0)
  const [pitch, setPitch] = useState(v.pitch || 0)
  const [styleNote, setStyleNote] = useState(v.style_note || '')
  const [err, setErr] = useState('')

  const mutation = useMutation({
    mutationFn: () =>
      api.updateVoice(v.id, {
        name: name.trim(),
        voice: voice.trim(),
        instruction: instruction.trim(),
        rate,
        pitch,
        style_note: styleNote.trim(),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['voices'] })
      void qc.invalidateQueries({ queryKey: ['series'] })
      void qc.invalidateQueries({ queryKey: ['seriesDetail'] })
      onClose()
    },
    onError: (e) => setErr((e as Error).message),
  })

  return (
    <Modal open onClose={onClose} title={`编辑声音 · ${v.name}${v.is_builtin ? '（内置）' : ''}`} wide>
      <form
        className="grid sm:grid-cols-2 gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          setErr('')
          mutation.mutate()
        }}
      >
        <Field label="名称">
          <TextInput value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="百炼音色 ID">
          <TextInput value={voice} onChange={(e) => setVoice(e.target.value)} placeholder="如 longtian_v3" />
        </Field>
        <Field label="风格指令">
          <TextInput
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            placeholder="如：沉稳、有书卷气、节奏从容"
          />
        </Field>
        <Field label="风格说明">
          <TextInput value={styleNote} onChange={(e) => setStyleNote(e.target.value)} />
        </Field>
        <Field label="语速（0.5-2.0，0=默认）">
          <input
            type="range"
            min="0"
            max="2"
            step="0.05"
            value={rate}
            onChange={(e) => setRate(parseFloat(e.target.value))}
            className="w-full"
          />
          <span className="text-xs text-paper-300/50">{rate.toFixed(2)}</span>
        </Field>
        <Field label="音高（0.5-2.0，0=默认）">
          <input
            type="range"
            min="0"
            max="2"
            step="0.05"
            value={pitch}
            onChange={(e) => setPitch(parseFloat(e.target.value))}
            className="w-full"
          />
          <span className="text-xs text-paper-300/50">{pitch.toFixed(2)}</span>
        </Field>
        {err && (
          <div className="sm:col-span-2">
            <ErrorBox>{err}</ErrorBox>
          </div>
        )}
        <div className="sm:col-span-2 flex gap-3">
          <Button type="submit" variant="seal" disabled={mutation.isPending}>
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
