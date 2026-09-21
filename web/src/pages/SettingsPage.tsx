import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { AppSettings } from '../types'
import { Button, Card, ErrorBox, Spinner } from '../components/ui'

const TABS = [
  { path: '/settings', label: '通用', exact: true },
  { path: '/settings/platforms', label: '平台账号' },
  { path: '/settings/ai', label: 'AI 模型' },
  { path: '/settings/publish', label: '发布' },
]

export default function SettingsPage() {
  const loc = useLocation()
  const activeTab = TABS.find((t) => t.exact ? loc.pathname === t.path : loc.pathname.startsWith(t.path)) ?? TABS[0]

  return (
    <div className="space-y-6">
      <nav className="text-sm text-paper-300/45 flex items-center gap-2">
        <Link to="/" className="hover:text-gold-500">首页</Link>
        <span>/</span>
        <span className="text-paper-300/80">设置</span>
      </nav>

      <h1 className="font-display text-2xl tracking-wider text-paper-100">设置</h1>

      {/* Tab 栏 */}
      <div className="flex gap-1 border-b border-ink-800">
        {TABS.map((tab) => (
          <Link
            key={tab.path}
            to={tab.path}
            className={`px-4 py-2 text-sm transition-colors border-b-2 -mb-px ${
              activeTab.path === tab.path
                ? 'border-gold-500 text-gold-500'
                : 'border-transparent text-paper-300/50 hover:text-paper-300/80'
            }`}
          >
            {tab.label}
          </Link>
        ))}
      </div>

      {/* Tab 内容 */}
      {activeTab.path === '/settings' && <GeneralTab />}
      {activeTab.path === '/settings/platforms' && <PlatformsTab />}
      {activeTab.path === '/settings/ai' && <AITab />}
      {activeTab.path === '/settings/publish' && <PublishTab />}
    </div>
  )
}

// ---- 通用设置 ----

function GeneralTab() {
  const { data: settings, isLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: api.getSettings,
  })

  if (isLoading) return <div className="flex justify-center py-12"><Spinner className="w-5 h-5" /></div>
  if (!settings) return null

  return (
    <Card title="通用设置">
      <div className="space-y-4 text-sm">
        <Field label="数据目录" value={settings.data_dir} disabled help="运行时数据根目录，修改需重启" />
        <Field label="bl 路径" value={settings.bl_bin} disabled help="百炼 CLI 可执行文件" />
        <Field label="ffmpeg 路径" value={settings.ffmpeg_bin} disabled help="视频处理工具" />
        <Field label="字幕字体" value={settings.subtitle_font} placeholder="留空自动探测" disabled help="中文字体路径" />
        <div className="flex items-center gap-3 pt-2">
          <Button
            variant="seal"
            className="px-4 py-1.5 text-xs"
            disabled
            title="通用设置为只读，修改请编辑 story.yaml"
          >
            通用设置为只读
          </Button>
        </div>
      </div>
    </Card>
  )
}

// ---- 平台账号（内嵌） ----

function PlatformsTab() {
  // 复用 PlatformsPage 的逻辑，但嵌在 settings 布局内
  return <PlatformsInline />
}

import PlatformsPage from './PlatformsPage'

function PlatformsInline() {
  return <PlatformsPage />
}

// ---- AI 模型设置 ----

function AITab() {
  const queryClient = useQueryClient()
  const [errMsg, setErrMsg] = useState('')
  const [saved, setSaved] = useState(false)

  const { data: settings, isLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: api.getSettings,
  })

  const mut = useMutation({
    mutationFn: (patch: Partial<AppSettings>) => api.updateSettings(patch),
    onError: (e) => { setErrMsg((e as Error).message); setSaved(false) },
    onSuccess: () => { setErrMsg(''); setSaved(true); void queryClient.invalidateQueries({ queryKey: ['settings'] }); setTimeout(() => setSaved(false), 2000) },
  })

  if (isLoading) return <div className="flex justify-center py-12"><Spinner className="w-5 h-5" /></div>
  if (!settings) return null

  return (
    <Card title="AI 模型与音色">
      {errMsg && <ErrorBox>{errMsg}</ErrorBox>}
      <div className="space-y-4 text-sm">
        <Field label="文本模型" value={settings.text_model} placeholder="留空用 bl 默认"
          onChange={(v) => mut.mutate({ text_model: v })} />
        <Field label="视频模型" value={settings.video_model} placeholder="留空用 bl 默认"
          onChange={(v) => mut.mutate({ video_model: v })} />
        <Field label="图片模型" value={settings.image_model} placeholder="留空用 bl 默认"
          onChange={(v) => mut.mutate({ image_model: v })} />
        <Field label="TTS 模型" value={settings.tts_model} placeholder="cosyvoice-v3-flash"
          onChange={(v) => mut.mutate({ tts_model: v })} />
        <Field label="TTS 音色" value={settings.tts_voice} placeholder="longtian_v3"
          onChange={(v) => mut.mutate({ tts_voice: v })} />
        <Field label="默认旁白指令" value={settings.tts_instruction} textarea
          onChange={(v) => mut.mutate({ tts_instruction: v })} />
        <Field label="百炼 API Key" value={settings.bailian_api_key ?? ''} placeholder="留空回退环境变量"
          onChange={(v) => mut.mutate({ bailian_api_key: v })} />
        <Field label="百炼 Base URL" value={settings.bailian_base_url} placeholder="https://dashscope.aliyuncs.com"
          onChange={(v) => mut.mutate({ bailian_base_url: v })} />
        <div className="flex items-center gap-3 pt-2">
          <Button variant="seal" className="px-4 py-1.5 text-xs" disabled={!mut.isPending && !saved}
            onClick={() => mut.mutate({})}>
            {mut.isPending ? '保存中…' : saved ? '✓ 已保存' : '保存'}
          </Button>
        </div>
      </div>
    </Card>
  )
}

// ---- 发布设置 ----

function PublishTab() {
  const queryClient = useQueryClient()
  const [errMsg, setErrMsg] = useState('')
  const [saved, setSaved] = useState(false)

  const { data: settings, isLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: api.getSettings,
  })

  const mut = useMutation({
    mutationFn: (patch: Partial<AppSettings>) => api.updateSettings(patch),
    onError: (e) => { setErrMsg((e as Error).message); setSaved(false) },
    onSuccess: () => { setErrMsg(''); setSaved(true); void queryClient.invalidateQueries({ queryKey: ['settings'] }); setTimeout(() => setSaved(false), 2000) },
  })

  if (isLoading) return <div className="flex justify-center py-12"><Spinner className="w-5 h-5" /></div>
  if (!settings) return null

  return (
    <Card title="发布设置">
      {errMsg && <ErrorBox>{errMsg}</ErrorBox>}
      <div className="space-y-4 text-sm">
        <Field label="sau 路径" value={settings.sau_bin} placeholder="sau"
          onChange={(v) => mut.mutate({ sau_bin: v })}
          help="social-auto-upload CLI 路径" />
        <Field label="Python 路径" value={settings.python_bin} placeholder="python3"
          onChange={(v) => mut.mutate({ python_bin: v })}
          help="sau 依赖的 Python 解释器" />
        <Field label="默认发布账号" value={settings.default_publish_account} placeholder="留空需显式指定 --account"
          onChange={(v) => mut.mutate({ default_publish_account: v })}
          help="发布时默认使用的账号名" />
        <Field label="B站默认分区 ID" value={String(settings.bilibili_default_tid || '')} placeholder="249（知识科普）"
          onChange={(v) => mut.mutate({ bilibili_default_tid: parseInt(v) || 0 })}
          help="B站投稿分区，249=知识科普" />
        <div className="flex items-center gap-3 pt-2">
          <Button variant="seal" className="px-4 py-1.5 text-xs" disabled={!mut.isPending && !saved}
            onClick={() => mut.mutate({})}>
            {mut.isPending ? '保存中…' : saved ? '✓ 已保存' : '保存'}
          </Button>
        </div>
      </div>
    </Card>
  )
}

// ---- 通用 Field 组件 ----

function Field({
  label,
  value,
  placeholder,
  disabled,
  help,
  textarea,
  onChange,
}: {
  label: string
  value: string
  placeholder?: string
  disabled?: boolean
  help?: string
  textarea?: boolean
  onChange?: (v: string) => void
}) {
  const [local, setLocal] = useState(value)
  const [dirty, setDirty] = useState(false)

  // 外部值变化时同步
  if (!dirty && local !== value) setLocal(value)

  return (
    <div className="grid grid-cols-[140px_1fr] gap-3 items-start">
      <div className="pt-2">
        <div className="text-paper-300/70">{label}</div>
        {help && <div className="text-[11px] text-paper-300/30 mt-0.5">{help}</div>}
      </div>
      {textarea ? (
        <textarea
          value={local}
          placeholder={placeholder}
          disabled={disabled}
          rows={3}
          onChange={(e) => { setLocal(e.target.value); setDirty(true) }}
          onBlur={() => { if (dirty && onChange) { onChange(local); setDirty(false) } }}
          className="px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 placeholder:text-paper-300/30 text-sm w-full resize-none font-body"
        />
      ) : (
        <input
          value={local}
          placeholder={placeholder}
          disabled={disabled}
          onChange={(e) => { setLocal(e.target.value); setDirty(true) }}
          onBlur={() => { if (dirty && onChange) { onChange(local); setDirty(false) } }}
          className="px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 placeholder:text-paper-300/30 text-sm w-full font-body disabled:opacity-50"
        />
      )}
    </div>
  )
}
