import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { AppSettings } from '../types'
import { Card, ErrorBox } from '../components/ui'

const TABS = [
  { path: '/settings', label: '通用', exact: true },
  { path: '/settings/ai', label: 'AI 模型' },
  { path: '/settings/providers', label: '供应商配置' },
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
      {activeTab.path === '/settings/providers' && <ProvidersTab />}
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
//
// 与「供应商配置」Tab 分离（2026-09-22）：
//  本 Tab = 默认实现——每个能力选一个默认供应商 + 全局默认旁白兜底，只管「用谁」。
//  凭据/模型在「供应商配置」Tab 填；未配置凭据的供应商在此不可选（禁用态）。
function AITab() {
  return (
    <div className="space-y-4">
      <Card title="默认实现">
        <p className="text-xs text-paper-300/45 leading-relaxed mb-4">
          选择各能力默认使用的供应商。灰色的尚未配置凭据，请先到
          <Link to="/settings/providers" className="text-gold-500 hover:underline mx-1">供应商配置</Link>
          填写后即可选。
        </p>
        <SettingFields
          fields={[
            { key: 'text_provider', label: '文本生成', type: 'provider-select', options: ['bailian', 'deepseek'] },
            { key: 'tts_provider', label: '语音合成', type: 'provider-select', options: ['bailian', 'minimax'] },
            { key: 'image_provider', label: '图片生成', type: 'provider-select', options: ['bailian', 'zhipu'] },
            { key: 'video_provider', label: '视频生成', type: 'provider-select', options: ['bailian', 'kling'] },
          ]}
        />
        <div className="mt-4 pt-4 border-t border-ink-800 space-y-3">
          <div>
            <div className="text-sm text-paper-100">默认旁白</div>
            <div className="text-[11px] text-paper-300/30 mt-0.5">
              系列未指定声音条目时的全局兜底音色与指令
            </div>
          </div>
          <SettingFields
            fields={[
              { key: 'tts_voice', label: '默认音色', type: 'voice-select' },
              { key: 'tts_instruction', label: '默认旁白指令', type: 'textarea', placeholder: '请用沉稳厚重、富有历史讲述感的语调…' },
            ]}
          />
        </div>
      </Card>
    </div>
  )
}

// ---- 供应商配置 ----
//
// 只管凭据与模型，与「谁是默认」无关；各供应商可同时配置、并存。
function ProvidersTab() {
  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-lg tracking-wider text-paper-100">供应商配置</h2>
        <p className="mt-1 text-xs text-paper-300/45">
          按供应商填写凭据与模型；可全部配好备用，配置完即可在「AI 模型」里选为默认。
        </p>
      </div>
      <ProviderConfigCards />
    </div>
  )
}

// ProviderConfigCards 各供应商的凭据与模型配置（与「谁是默认」无关）。
function ProviderConfigCards() {
  return (
    <>
      <Card title="阿里云百炼 (bl)">
        <SettingFields
          fields={[
            { key: 'bailian_api_key', label: 'API Key', type: 'password', placeholder: '留空回退环境变量', help: '或设置 STORY_BAILIAN_API_KEY' },
            { key: 'bailian_base_url', label: 'Base URL', type: 'text', placeholder: 'https://dashscope.aliyuncs.com', help: '留空用默认地址' },
            { key: 'text_model', label: '文本模型', type: 'select', options: ['qwen-max', 'qwen-plus', 'qwen-turbo', 'qwen-max-latest'], help: '留空用 bl 默认', allowEmpty: true },
            { key: 'tts_model', label: 'TTS 模型', type: 'select', options: ['cosyvoice-v3-flash', 'cosyvoice-v3-plus', 'cosyvoice-v3.5-plus', 'cosyvoice-v3.5-flash'] },
            { key: 'image_model', label: '图片模型', type: 'select', options: ['wanx2.1-t2i-turbo', 'wanx2.1-t2i-plus', 'wanx2.1-t2i-max'], help: '小人书模式 / 定妆照' },
            { key: 'video_model', label: '视频模型', type: 'select', options: ['wan3.0-video', 'wanx-video'], help: '视频模式时使用' },
          ]}
        />
      </Card>

      <Card title="DeepSeek">
        <SettingFields
          fields={[
            { key: 'deepseek_api_key', label: 'API Key', type: 'password', placeholder: 'sk-…', help: '或设置 DEEPSEEK_API_KEY' },
            { key: 'deepseek_base_url', label: 'API 地址', type: 'text', placeholder: 'https://api.deepseek.com', help: '留空用默认地址' },
            { key: 'deepseek_model', label: '模型', type: 'select', options: ['deepseek-chat', 'deepseek-reasoner'], help: 'deepseek-chat 通用，deepseek-reasoner 推理更强' },
          ]}
        />
      </Card>

      <Card title="MiniMax">
        <SettingFields
          fields={[
            { key: 'minimax_api_key', label: 'API Key', type: 'password', placeholder: 'eyJ…', help: '或设置 MINIMAX_API_KEY' },
            { key: 'minimax_base_url', label: 'API 地址', type: 'text', placeholder: 'https://api.minimax.chat', help: '留空用默认地址' },
            { key: 'minimax_model', label: '模型', type: 'select', options: ['speech-02-hd', 'speech-01-hd', 'speech-01'], help: 'speech-02-hd 最新最自然' },
          ]}
        />
      </Card>

      <Card title="智谱 CogView">
        <SettingFields
          fields={[
            { key: 'zhipu_api_key', label: 'API Key', type: 'password', placeholder: '…', help: '或设置 ZHIPU_API_KEY' },
            { key: 'zhipu_base_url', label: 'API 地址', type: 'text', placeholder: 'https://open.bigmodel.cn/api/paas/v4', help: '留空用默认地址' },
          ]}
        />
      </Card>

      <Card title="可灵 Kling">
        <SettingFields
          fields={[
            { key: 'kling_access_key', label: 'Access Key', type: 'password', placeholder: '…', help: '或设置 KLING_ACCESS_KEY' },
            { key: 'kling_secret_key', label: 'Secret Key', type: 'password', placeholder: '…', help: '或设置 KLING_SECRET_KEY' },
            { key: 'kling_base_url', label: 'API 地址', type: 'text', placeholder: 'https://api.klingai.com', help: '留空用默认地址' },
          ]}
        />
      </Card>
    </>
  )
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
  | { type: 'voice-select'; options?: never; parse?: never; allowEmpty?: never }
)

// VoiceSelectField 声音下拉选择器：从声音列表 API 加载，显示声音名称 + 音色 ID。
function VoiceSelectField({ label, value, onChange }: {
  label: string
  value: string
  onChange: (v: string) => void
}) {
  const { data: voices } = useQuery({
    queryKey: ['voices'],
    queryFn: api.listVoices,
    staleTime: Infinity,
  })
  return (
    <div className="grid grid-cols-[140px_1fr] gap-3 items-center">
      <label className="text-paper-300/70">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="px-3 py-1.5 rounded bg-ink-800 border border-ink-700 text-paper-100 text-sm font-body min-w-[200px]"
      >
        <option value="">未设置</option>
        {voices?.map((v) => (
          <option key={v.id} value={v.id}>
            {v.name}（{v.voice}）
          </option>
        ))}
      </select>
    </div>
  )
}

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

  // 已配置凭据的供应商（后端下发）；为空表示后端未提供该信息，此时一律不禁用。
  const readyProviders = settings.ready_providers ?? []
  const readySet = new Set(readyProviders)

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
            <div key={f.key} className="grid grid-cols-[140px_1fr] gap-3 items-start">
              <label className="text-paper-300/70 pt-2">{f.label}</label>
              <div>
                <RadioGroup
                  value={val || 'bailian'}
                  options={(f.options || []).map((opt) => ({
                    value: opt,
                    label: providerDescs[opt]?.label ?? opt,
                    desc: providerDescs[opt]?.desc,
                    // 未配置凭据的供应商禁用，引导去「供应商配置」填写。
                    disabled: readyProviders.length > 0 && !readySet.has(opt),
                  }))}
                  onChange={(v) => mut.mutate({ [f.key]: v })}
                />
                {readyProviders.length > 0 && val && !readySet.has(val) && (
                  <div className="mt-1.5 text-[11px] text-amber-400/80">
                    当前默认「{providerDescs[val]?.label ?? val}」尚未配置凭据，调用会失败；请到「供应商配置」补齐。
                  </div>
                )}
              </div>
            </div>
          )
        }
        if (f.type === 'voice-select') {
          return (
            <VoiceSelectField
              key={f.key}
              label={f.label}
              value={val}
              onChange={(v) => mut.mutate({ [f.key]: v })}
            />
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
  options: { value: string; label: string; desc?: string; disabled?: boolean }[]
  onChange: (v: string) => void
}) {
  return (
    <div className="flex gap-3">
      {options.map((opt) => {
        const selected = value === opt.value
        return (
          <label
            key={opt.value}
            title={opt.disabled ? '尚未配置该供应商的凭据，请先到「供应商配置」填写' : undefined}
            className={`flex items-center gap-2 px-3 py-2 rounded-lg border transition-colors ${
              opt.disabled
                ? selected
                  ? 'border-gold-500/30 bg-gold-500/5 text-gold-500/60 cursor-not-allowed'
                  : 'border-ink-800 bg-ink-900/20 text-paper-300/25 cursor-not-allowed'
                : selected
                  ? 'border-gold-500/50 bg-gold-500/10 text-gold-500 cursor-pointer'
                  : 'border-ink-700 bg-ink-900/30 text-paper-300/60 hover:border-ink-600 cursor-pointer'
            }`}
          >
            <input
              type="radio"
              checked={selected}
              disabled={opt.disabled}
              onChange={() => onChange(opt.value)}
              className="sr-only"
            />
            <div className="w-3.5 h-3.5 rounded-full border-2 flex items-center justify-center shrink-0"
              style={{ borderColor: selected ? '#d4a853' : opt.disabled ? '#2a2a2a' : '#3a3a3a' }}>
              {selected && <div className="w-2 h-2 rounded-full bg-gold-500" />}
            </div>
            <div>
              <div className="text-sm">
                {opt.label}
                {opt.disabled && <span className="ml-1 text-[11px]">未配置</span>}
              </div>
              {opt.desc && <div className="text-[11px] text-paper-300/40">{opt.desc}</div>}
            </div>
          </label>
        )
      })}
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
