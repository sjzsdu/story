import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { AppSettings } from '../types'
import { Card, ErrorBox } from '../components/ui'

const TABS = [
  { path: '/settings', label: '通用', exact: true },
  { path: '/settings/ai', label: 'AI 模型' },
  { path: '/settings/platforms', label: '平台账号' },
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
      {activeTab.path === '/settings/ai' && <AITab />}
      {activeTab.path === '/settings/platforms' && <PlatformsTab />}
      {activeTab.path === '/settings/publish' && <PublishTab />}
    </div>
  )
}

// ---- 通用设置 ----

function GeneralTab() {
  return (
    <div className="space-y-4">
      <Card title="运行环境">
        <div className="space-y-3 text-sm">
          <InfoRow label="数据目录" value="data/" help="运行时数据根目录（数据库与媒体文件）" />
          <InfoRow label="bl 路径" value="bl" help="百炼 CLI 可执行文件" />
          <InfoRow label="ffmpeg 路径" value="ffmpeg" help="视频处理工具" />
        </div>
      </Card>
      <Card title="默认产出">
        <SettingFields
          fields={[
            { key: 'default_ratio', label: '默认画面比例', type: 'select', options: ['9:16', '16:9', '1:1', '3:4'] },
            { key: 'default_resolution', label: '默认分辨率', type: 'select', options: ['720P', '1080P'] },
          ]}
        />
      </Card>
      <Card title="并发与重试">
        <SettingFields
          fields={[
            { key: 'max_concurrency', label: '单集并发镜头数', type: 'select', options: ['1', '2', '3', '4', '5'], parse: Number },
            { key: 'max_retries', label: '单镜头重试次数', type: 'select', options: ['1', '2', '3', '5'], parse: Number },
          ]}
        />
      </Card>
      <Card title="字幕">
        <SettingFields
          fields={[
            { key: 'subtitle_font', label: '字幕字体', type: 'text', placeholder: '留空自动探测系统中文字体', help: '中文字体路径' },
          ]}
        />
      </Card>
    </div>
  )
}

// ---- AI 模型设置 ----

function AITab() {
  return (
    <div className="space-y-4">
      {/* 文本生成 */}
      <Card title="文本生成">
        <SettingFields
          fields={[
            { key: 'text_provider', label: 'Provider', type: 'provider-select', options: ['bailian', 'deepseek'] },
          ]}
        />
        <ProviderConfig group="text" />
      </Card>

      {/* 语音合成 */}
      <Card title="语音合成 (TTS)">
        <SettingFields
          fields={[
            { key: 'tts_provider', label: 'Provider', type: 'provider-select', options: ['bailian', 'minimax'] },
          ]}
        />
        <ProviderConfig group="tts" />
      </Card>

      {/* 图片生成 */}
      <Card title="图片生成">
        <SettingFields
          fields={[
            { key: 'image_provider', label: 'Provider', type: 'provider-select', options: ['bailian', 'zhipu'] },
          ]}
        />
        <ProviderConfig group="image" />
      </Card>

      {/* 视频生成 */}
      <Card title="视频生成">
        <SettingFields
          fields={[
            { key: 'video_provider', label: 'Provider', type: 'provider-select', options: ['bailian', 'kling'] },
          ]}
        />
        <ProviderConfig group="video" />
      </Card>
    </div>
  )
}

// 根据选中的 provider 动态显示对应配置
function ProviderConfig({ group }: { group: 'text' | 'tts' | 'image' | 'video' }) {
  const { data: settings } = useQuery({ queryKey: ['settings'], queryFn: api.getSettings })
  if (!settings) return null

  const provider = group === 'text' ? (settings.text_provider || 'bailian')
    : group === 'tts' ? (settings.tts_provider || 'bailian')
    : group === 'image' ? (settings.image_provider || 'bailian')
    : (settings.video_provider || 'bailian')

  // Bailian 配置（所有 group 共用）
  if (provider === 'bailian') {
    if (group === 'text') {
      return (
        <div className="mt-4 pt-4 border-t border-ink-800">
          <SettingFields
            fields={[
              { key: 'text_model', label: '文本模型', type: 'select', options: ['qwen-max', 'qwen-plus', 'qwen-turbo', 'qwen-max-latest'], help: '留空用 bl 默认', allowEmpty: true },
              { key: 'bailian_api_key', label: 'API Key', type: 'password', placeholder: '留空回退环境变量', help: '或设置 STORY_BAILIAN_API_KEY' },
              { key: 'bailian_base_url', label: 'Base URL', type: 'text', placeholder: 'https://dashscope.aliyuncs.com', help: '留空用默认地址' },
            ]}
          />
        </div>
      )
    }
    if (group === 'tts') {
      return (
        <div className="mt-4 pt-4 border-t border-ink-800">
          <SettingFields
            fields={[
              { key: 'tts_model', label: 'TTS 模型', type: 'select', options: ['cosyvoice-v3-flash', 'cosyvoice-v3-plus', 'cosyvoice-v3.5-plus', 'cosyvoice-v3.5-flash'] },
              { key: 'tts_voice', label: '默认音色', type: 'text', placeholder: 'longtian_v3' },
              { key: 'tts_instruction', label: '默认旁白指令', type: 'textarea', placeholder: '请用沉稳厚重、富有历史讲述感的语调…' },
            ]}
          />
        </div>
      )
    }
    if (group === 'image') {
      return (
        <div className="mt-4 pt-4 border-t border-ink-800">
          <SettingFields
            fields={[
              { key: 'image_model', label: '图片模型', type: 'select', options: ['wanx2.1-t2i-turbo', 'wanx2.1-t2i-plus', 'wanx2.1-t2i-max'], help: '小人书模式 / 定妆照' },
            ]}
          />
        </div>
      )
    }
    // video
    return (
      <div className="mt-4 pt-4 border-t border-ink-800">
        <SettingFields
          fields={[
            { key: 'video_model', label: '视频模型', type: 'select', options: ['wan3.0-video', 'wanx-video'], help: '视频模式时使用' },
          ]}
        />
      </div>
    )
  }

  // DeepSeek
  if (provider === 'deepseek' && group === 'text') {
    return (
      <div className="mt-4 pt-4 border-t border-ink-800">
        <SettingFields
          fields={[
            { key: 'deepseek_api_key', label: 'API Key', type: 'password', placeholder: 'sk-…', help: '或设置 DEEPSEEK_API_KEY' },
            { key: 'deepseek_base_url', label: 'API 地址', type: 'text', placeholder: 'https://api.deepseek.com', help: '留空用默认地址' },
            { key: 'deepseek_model', label: '模型', type: 'select', options: ['deepseek-chat', 'deepseek-reasoner'], help: 'deepseek-chat 通用，deepseek-reasoner 推理更强' },
          ]}
        />
      </div>
    )
  }

  // MiniMax TTS
  if (provider === 'minimax' && group === 'tts') {
    return (
      <div className="mt-4 pt-4 border-t border-ink-800">
        <SettingFields
          fields={[
            { key: 'minimax_api_key', label: 'API Key', type: 'password', placeholder: 'eyJ…', help: '或设置 MINIMAX_API_KEY' },
            { key: 'minimax_base_url', label: 'API 地址', type: 'text', placeholder: 'https://api.minimax.chat', help: '留空用默认地址' },
            { key: 'minimax_model', label: '模型', type: 'select', options: ['speech-02-hd', 'speech-01-hd', 'speech-01'], help: 'speech-02-hd 最新最自然' },
          ]}
        />
      </div>
    )
  }

  // 智谱 CogView
  if (provider === 'zhipu' && group === 'image') {
    return (
      <div className="mt-4 pt-4 border-t border-ink-800">
        <SettingFields
          fields={[
            { key: 'zhipu_api_key', label: 'API Key', type: 'password', placeholder: '…', help: '或设置 ZHIPU_API_KEY' },
            { key: 'zhipu_base_url', label: 'API 地址', type: 'text', placeholder: 'https://open.bigmodel.cn/api/paas/v4', help: '留空用默认地址' },
          ]}
        />
      </div>
    )
  }

  // 可灵 Kling
  if (provider === 'kling' && group === 'video') {
    return (
      <div className="mt-4 pt-4 border-t border-ink-800">
        <SettingFields
          fields={[
            { key: 'kling_access_key', label: 'Access Key', type: 'password', placeholder: '…', help: '或设置 KLING_ACCESS_KEY' },
            { key: 'kling_secret_key', label: 'Secret Key', type: 'password', placeholder: '…', help: '或设置 KLING_SECRET_KEY' },
            { key: 'kling_base_url', label: 'API 地址', type: 'text', placeholder: 'https://api.klingai.com', help: '留空用默认地址' },
          ]}
        />
      </div>
    )
  }

  return null
}

// ---- 平台账号 ----

function PlatformsTab() {
  return <PlatformsInline />
}

import PlatformsPage from './PlatformsPage'

function PlatformsInline() {
  return <PlatformsPage />
}

// ---- 发布设置 ----

function PublishTab() {
  return (
    <Card title="发布设置">
      <SettingFields
        fields={[
          { key: 'sau_bin', label: 'sau 路径', type: 'text', placeholder: 'sau', help: 'social-auto-upload CLI 路径' },
          { key: 'python_bin', label: 'Python 路径', type: 'text', placeholder: 'python3', help: 'sau 依赖的 Python 解释器' },
          { key: 'default_publish_account', label: '默认发布账号', type: 'text', placeholder: '留空需显式指定 --account', help: '发布时默认使用的账号名' },
          { key: 'bilibili_default_tid', label: 'B站默认分区 ID', type: 'text', placeholder: '249（知识科普）', help: 'B站投稿分区' },
        ]}
      />
    </Card>
  )
}

// ---- 通用组件 ----

type FieldDef = {
  key: string
  label: string
  help?: string
  placeholder?: string
} & (
  | { type: 'text' | 'password' | 'textarea'; options?: never; parse?: never; allowEmpty?: never }
  | { type: 'select'; options: string[]; parse?: (v: string) => any; allowEmpty?: boolean }
  | { type: 'provider-select'; options: string[]; parse?: never; allowEmpty?: never }
)

function SettingFields({ fields }: { fields: FieldDef[] }) {
  const queryClient = useQueryClient()
  const { data: settings } = useQuery({ queryKey: ['settings'], queryFn: api.getSettings })
  const [errMsg, setErrMsg] = useState('')
  const [savedKey, setSavedKey] = useState('')

  const mut = useMutation({
    mutationFn: (patch: Partial<AppSettings>) => api.updateSettings(patch),
    onError: (e) => { setErrMsg((e as Error).message) },
    onSuccess: (_, vars) => {
      setErrMsg('')
      const key = Object.keys(vars)[0]
      setSavedKey(key)
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
      setTimeout(() => setSavedKey(''), 1500)
    },
  })

  if (!settings) return null

  return (
    <div className="space-y-3 text-sm">
      {errMsg && <ErrorBox>{errMsg}</ErrorBox>}
      {fields.map((f) => {
        const val = (settings as any)[f.key] ?? ''
        if (f.type === 'provider-select') {
          const providerDescs: Record<string, { label: string; desc: string }> = {
            bailian: { label: '百炼 (bl)', desc: '阿里云百炼平台，CLI 驱动' },
            deepseek: { label: 'DeepSeek', desc: 'DeepSeek API，HTTP 直连' },
            minimax: { label: 'MiniMax', desc: 'MiniMax API，中文语音最自然' },
            zhipu: { label: '智谱 CogView', desc: '智谱 API，中文 prompt 友好' },
            kling: { label: '可灵 Kling', desc: '快手 API，中文视频最强' },
          }
          return (
            <div key={f.key} className="grid grid-cols-[140px_1fr] gap-3 items-center">
              <label className="text-paper-300/70">{f.label}</label>
              <RadioGroup
                value={val || 'bailian'}
                options={(f.options || []).map((opt) => ({
                  value: opt,
                  label: providerDescs[opt]?.label ?? opt,
                  desc: providerDescs[opt]?.desc,
                }))}
                onChange={(v) => mut.mutate({ [f.key]: v })}
              />
            </div>
          )
        }
        if (f.type === 'select') {
          return (
            <div key={f.key} className="grid grid-cols-[140px_1fr] gap-3 items-center">
              <div>
                <label className="text-paper-300/70">{f.label}</label>
                {f.help && <div className="text-[11px] text-paper-300/30 mt-0.5">{f.help}</div>}
              </div>
              <div className="flex items-center gap-2">
                <Select
                  value={val}
                  options={f.options!}
                  allowEmpty={f.allowEmpty}
                  placeholder={f.placeholder}
                  onChange={(v) => {
                    const parsed = f.parse ? f.parse(v) : v
                    mut.mutate({ [f.key]: parsed })
                  }}
                />
                {savedKey === f.key && <span className="text-xs text-emerald-400">✓</span>}
              </div>
            </div>
          )
        }
        if (f.type === 'textarea') {
          return (
            <TextareaField
              key={f.key}
              label={f.label}
              help={f.help}
              value={val}
              placeholder={f.placeholder}
              saved={savedKey === f.key}
              onChange={(v) => mut.mutate({ [f.key]: v })}
            />
          )
        }
        // text / password
        return (
          <PasswordFieldOrText
            key={f.key}
            field={f}
            value={val}
            saved={savedKey === f.key}
            onChange={(v) => mut.mutate({ [f.key]: v })}
          />
        )
      })}
    </div>
  )
}

function RadioGroup({ value, options, onChange }: {
  value: string
  options: { value: string; label: string; desc?: string }[]
  onChange: (v: string) => void
}) {
  return (
    <div className="flex gap-3">
      {options.map((opt) => (
        <label
          key={opt.value}
          className={`flex items-center gap-2 px-3 py-2 rounded-lg border cursor-pointer transition-colors ${
            value === opt.value
              ? 'border-gold-500/50 bg-gold-500/10 text-gold-500'
              : 'border-ink-700 bg-ink-900/30 text-paper-300/60 hover:border-ink-600'
          }`}
        >
          <input
            type="radio"
            checked={value === opt.value}
            onChange={() => onChange(opt.value)}
            className="sr-only"
          />
          <div className="w-3.5 h-3.5 rounded-full border-2 flex items-center justify-center shrink-0"
            style={{ borderColor: value === opt.value ? '#d4a853' : '#3a3a3a' }}>
            {value === opt.value && <div className="w-2 h-2 rounded-full bg-gold-500" />}
          </div>
          <div>
            <div className="text-sm">{opt.label}</div>
            {opt.desc && <div className="text-[11px] text-paper-300/40">{opt.desc}</div>}
          </div>
        </label>
      ))}
    </div>
  )
}

function Select({ value, options, allowEmpty, placeholder, onChange }: {
  value: string
  options: string[]
  allowEmpty?: boolean
  placeholder?: string
  onChange: (v: string) => void
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 text-sm font-body min-w-[200px]"
    >
      {allowEmpty && <option value="">{placeholder || '— 不指定 —'}</option>}
      {options.map((opt) => (
        <option key={opt} value={opt}>{opt}</option>
      ))}
    </select>
  )
}

function PasswordFieldOrText({ field, value, saved, onChange }: {
  field: { key: string; label: string; placeholder?: string; help?: string }
  value: string
  saved: boolean
  onChange: (v: string) => void
}) {
  const [local, setLocal] = useState(value)
  const [dirty, setDirty] = useState(false)
  if (!dirty && local !== value) setLocal(value)

  return (
    <div className="grid grid-cols-[140px_1fr] gap-3 items-center">
      <div>
        <label className="text-paper-300/70">{field.label}</label>
        {field.help && <div className="text-[11px] text-paper-300/30 mt-0.5">{field.help}</div>}
      </div>
      <div className="flex items-center gap-2">
        <input
          type="password"
          value={local}
          placeholder={field.placeholder}
          onChange={(e) => { setLocal(e.target.value); setDirty(true) }}
          onBlur={() => { if (dirty) { onChange(local); setDirty(false) } }}
          className="px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 placeholder:text-paper-300/30 text-sm w-full font-body"
        />
        {saved && <span className="text-xs text-emerald-400">✓</span>}
      </div>
    </div>
  )
}

function TextareaField({ label, help, value, placeholder, saved, onChange }: {
  label: string
  help?: string
  value: string
  placeholder?: string
  saved: boolean
  onChange: (v: string) => void
}) {
  const [local, setLocal] = useState(value)
  const [dirty, setDirty] = useState(false)
  if (!dirty && local !== value) setLocal(value)

  return (
    <div className="grid grid-cols-[140px_1fr] gap-3 items-start">
      <div className="pt-2">
        <label className="text-paper-300/70">{label}</label>
        {help && <div className="text-[11px] text-paper-300/30 mt-0.5">{help}</div>}
      </div>
      <div className="flex items-start gap-2">
        <textarea
          value={local}
          placeholder={placeholder}
          rows={3}
          onChange={(e) => { setLocal(e.target.value); setDirty(true) }}
          onBlur={() => { if (dirty) { onChange(local); setDirty(false) } }}
          className="px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 placeholder:text-paper-300/30 text-sm w-full resize-none font-body"
        />
        {saved && <span className="text-xs text-emerald-400 mt-2">✓</span>}
      </div>
    </div>
  )
}

function InfoRow({ label, value, help }: { label: string; value: string; help?: string }) {
  return (
    <div className="grid grid-cols-[140px_1fr] gap-3 items-center">
      <div>
        <div className="text-paper-300/70">{label}</div>
        {help && <div className="text-[11px] text-paper-300/30 mt-0.5">{help}</div>}
      </div>
      <div className="text-paper-300/50 font-body text-sm">{value}</div>
    </div>
  )
}
