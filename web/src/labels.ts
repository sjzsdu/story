// 跨页共享的用户可见术语与选项：同一概念只在这里定义一次，避免各页各写一套造成文案漂移。

import type { CapabilityCatalog, CapabilityInfo } from './types'

// 画面模式（SeriesConfig.visual_mode）的中文名。
export function visualModeLabel(mode: string | undefined): string {
  return mode === 'video' ? 'AI 视频' : '小人书插画'
}

// TTS 供应商（Voice.Provider）的中文名。选项由后端 /api/voice-providers 下发，
// 这里只负责展示名；接入新供应商时补一条即可，未登记则回退显示原始 id。
const VOICE_PROVIDER_LABELS: Record<string, string> = {
  bailian: '阿里云百炼 CosyVoice',
  minimax: 'MiniMax',
}

export function voiceProviderLabel(id: string | undefined): string {
  const key = (id ?? '').trim()
  if (!key) return '阿里云百炼 CosyVoice'
  return VOICE_PROVIDER_LABELS[key] ?? key
}

// 画幅与分辨率档位：新建系列与系列设置共用。
export const RATIO_OPTIONS = ['9:16', '16:9', '1:1', '3:4']
export const RESOLUTION_OPTIONS = ['1080P', '720P']

// ---- Provider 字段（系列覆盖 / 系统默认）----
//
// 这里只保留「字段 → 标签」映射：可选项与供应商展示文案一律来自后端能力目录
//（GET /api/capabilities），前端不再硬编码供应商清单——接入新供应商零前端改动。
// SeriesProviderFieldKey 有系列级覆盖的四个能力（§18/§21）；系列创建/编辑表单
// 以它为键渲染，键集合必须与后端 SeriesConfig 的 provider 字段严格一致。
export type SeriesProviderFieldKey =
  | 'text_provider'
  | 'tts_provider'
  | 'image_provider'
  | 'video_provider'

// ProviderFieldKey 全集：系列覆盖四字段 + §22 三个新能力的系统默认字段
//（新能力无系列覆盖，只出现在设置页）。
export type ProviderFieldKey = SeriesProviderFieldKey
  | 'image_understand_provider'
  | 'video_understand_provider'
  | 'sfx_provider'

export const PROVIDER_FIELDS: { key: SeriesProviderFieldKey; label: string }[] = [
  { key: 'text_provider', label: '文本生成' },
  { key: 'tts_provider', label: '语音合成' },
  { key: 'image_provider', label: '图片生成' },
  { key: 'video_provider', label: '视频生成' },
]

// SYSTEM_PROVIDER_FIELDS 系统默认（story.yaml 级）能力字段：PROVIDER_FIELDS 的
// 四项 + §22 三个新能力（图像理解 / 视频理解 / 音效生成）。
// 新能力**没有系列级覆盖**（后端 SeriesField 为空），故它们只出现在设置页，
// 不进 PROVIDER_FIELDS——后者被系列创建/编辑表单复用，多渲染不存在的系列字段会提交非法键。
export const SYSTEM_PROVIDER_FIELDS: { key: ProviderFieldKey; label: string }[] = [
  ...PROVIDER_FIELDS,
  { key: 'image_understand_provider', label: '图像理解' },
  { key: 'video_understand_provider', label: '视频理解' },
  { key: 'sfx_provider', label: '音效生成' },
]

// capabilityForField 按配置字段找对应能力项（text/board/plan 共用 text_provider，
// 目录里「故事生成」排在最前，find 命中的就是它——三项的可选项本就同源）。
export function capabilityForField(
  catalog: CapabilityCatalog | undefined,
  configField: string,
): CapabilityInfo | undefined {
  if (!configField) return undefined
  return catalog?.capabilities.find((c) => c.config_field === configField)
}

// providerOptions 某字段的下拉选项：首项恒为「系统默认」（空值），其后是能力目录
// 下发的已登记实现。目录未加载时只返回「系统默认」，不闪错。
export function providerOptions(
  catalog: CapabilityCatalog | undefined,
  configField: string,
): { value: string; label: string; desc?: string }[] {
  const opts: { value: string; label: string; desc?: string }[] = [
    { value: '', label: '系统默认', desc: '跟随全局默认实现' },
  ]
  const cap = capabilityForField(catalog, configField)
  if (!cap) return opts
  for (const p of cap.providers) opts.push({ value: p.key, label: p.label, desc: p.desc })
  return opts
}

// providerLabel 渲染某字段当前取值的展示名：空＝「系统默认」；
// 未在选项里的历史值原样显示（不静默藏掉用户的旧配置）。
export function providerLabel(
  value: string | undefined,
  options: { value: string; label: string }[],
): string {
  const v = (value ?? '').trim()
  if (!v) return '系统默认'
  return options.find((o) => o.value === v)?.label ?? v
}

// 时间戳展示：精确到分钟，本地时区。
export function formatTime(iso: string | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}