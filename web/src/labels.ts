// 跨页共享的用户可见术语与选项：同一概念只在这里定义一次，避免各页各写一套造成文案漂移。

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

// 系列级 Provider 覆盖字段：新建系列与系列设置共用同一份定义（键、标签、可选项）。
export const PROVIDER_FIELDS: {
  key: 'text_provider' | 'tts_provider' | 'image_provider' | 'video_provider'
  label: string
  options: { value: string; label: string }[]
}[] = [
  {
    key: 'text_provider',
    label: '文本生成',
    options: [
      { value: '', label: '系统默认' },
      { value: 'bailian', label: '百炼 (bl)' },
      { value: 'deepseek', label: 'DeepSeek' },
    ],
  },
  {
    key: 'tts_provider',
    label: '语音合成',
    options: [
      { value: '', label: '系统默认' },
      { value: 'bailian', label: '百炼 (CosyVoice)' },
      { value: 'minimax', label: 'MiniMax' },
    ],
  },
  {
    key: 'image_provider',
    label: '图片生成',
    options: [
      { value: '', label: '系统默认' },
      { value: 'bailian', label: '百炼 (通义万相)' },
      { value: 'zhipu', label: '智谱 (CogView)' },
    ],
  },
  {
    key: 'video_provider',
    label: '视频生成',
    options: [
      { value: '', label: '系统默认' },
      { value: 'bailian', label: '百炼 (Wanx Video)' },
      { value: 'kling', label: '可灵 (Kling)' },
    ],
  },
]

// Provider 覆盖值的中文名（展示用；空值＝系统默认）。
export function providerLabel(key: string, value: string | undefined): string {
  const field = PROVIDER_FIELDS.find((f) => f.key === key)
  return field?.options.find((o) => o.value === (value ?? ''))?.label ?? '系统默认'
}

// 时间戳展示：精确到分钟，本地时区。
export function formatTime(iso: string | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}