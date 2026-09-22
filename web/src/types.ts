// 与 Go internal/domain 的 JSON 结构一一对应。

// §17 版本树：一集由四个阶段依次派生，每个阶段可有多份版本（Attempt 递增）。
export type Stage = 'story' | 'storyboard' | 'media' | 'final'
export type NodeStatus = 'pending' | 'running' | 'done' | 'failed'

export interface StoryCandidate {
  index: number
  title: string
  dynasty: string
  source: string
  summary: string
  content: string
}

export interface Scene {
  id: number
  visual_prompt: string
  narration: string
  duration: number
  camera: string
}

export interface Storyboard {
  refs?: VisualRef[]
  scenes: Scene[]
}

export interface MediaResult {
  scene_id: number
  path: string
  duration_sec: number
  skipped?: boolean
  err?: string
}

// VersionNode 版本树上的一个节点：某阶段在一组派生输入下的一次产出。
// id 即派生键（内容寻址），上游输入一变 id 与产物目录就变，旧产物天然不会被复用。
export interface VersionNode {
  id: string
  stage: Stage
  parent_id?: string
  attempt: number // 同一组派生输入下的第 n 次尝试（0 起）；>0 即「换一版」
  runs: number // 本节点被执行次数（失败重试与续跑累加）
  // note 本版附加要求：「重做（换一版）」时用户填的迭代方向，只作用于这一版。
  note?: string
  status: NodeStatus
  error?: string
  dir: string
  story?: StoryCandidate
  storyboard?: Storyboard
  refs?: VisualRef[]
  clips?: MediaResult[]
  audios?: MediaResult[]
  outputs?: string[]
  created_at: string
  updated_at: string
}

export interface SeriesConfig {
  dynasty: string
  ratio: string
  resolution: string
  video_style: string
  visual_mode: string // comic（小人书插画+运镜，默认）/ video（AI 视频）
  tts_voice: string
  tts_instruction: string
  tts_rate?: number
  tts_pitch?: number
  voice_profile?: string // 内置声音 key：longtian/longze/longcheng/longfei/longhao/longxiaoxia
  target_platforms?: string[]
  max_concurrency: number
  max_retries: number
  creative?: CreativeStyle // 创作控制参数（全部 omitempty，未设置即不出现）
}

// 创作控制参数：字段名即 GET /api/creative-catalog 里的 knob key。
// 全部可空：未设置（undefined 或空串）＝跟随内置默认，产出与历史行为一致。
// 注：video_style 在后端响应里位于 config.video_style（独立字段），但在请求体的
// creative map 中它与其它参数同列——前端统一按 knob 渲染，故这里一并包含。
export interface CreativeStyle {
  preset?: string
  narrative?: string
  audience?: string
  length?: string
  motion?: string
  video_style?: string
  instruction?: string
}

// ---- 创作参数注册表（后端 templates.Catalog() 的 JSON 快照） ----
// 前端严禁硬编码任何参数名/选项，渲染完全由这些结构驱动。

export interface CreativeOption {
  key: string
  label: string
}

export interface CreativeKnob {
  key: string
  label: string
  help: string
  default_label: string // 默认（空值）的中文名，保持后端 snake_case
  type: 'enum' | 'text'
  max_length?: number // 文本型参数的最大字符数（后端给出，前端不写死）
  options: CreativeOption[] // text 时为空数组（后端保证非 null）
}

export interface CreativePreset {
  key: string
  name: string
  desc: string
  values: Record<string, string> // {knobKey: value}，未列出的一律回落默认
}

export interface CreativeCatalog {
  knobs: CreativeKnob[]
  presets: CreativePreset[]
  default_preset: string
}

// §16：声音顶层实体（与 Series 同级）。series.voice_id 创建后锁定不可改。
export interface Voice {
  id: string
  name: string
  provider: string // TTS 供应商标识，如 bailian
  voice: string // 该供应商体系内音色 ID，百炼下如 longtian_v3
  model?: string // 驱动模型：造声（声音设计/复刻）出来的音色必填，普通音色留空
  instruction?: string
  rate?: number
  pitch?: number
  style_note?: string
  is_builtin: boolean
  created_at: string
  updated_at: string
}

// SystemVoice 供应商系统音色（浏览音色库挑选，不落库）。
export interface SystemVoice {
  id: string
  name: string
  description: string
  language: string
}

// VoiceProfile 旧值对象类型（兼容期保留，server preview/试音仍接收）。
export interface VoiceProfile {
  key?: string
  name: string
  provider?: string
  voice: string
  instruction?: string
  rate?: number
  pitch?: number
  style_note?: string
}

export interface CharacterSetting {
  name: string
  identity: string
  appearance: string
  temperament: string
  ref_image?: string
}

// 视觉参考：对图片/视频生成的一致性约束（character 人物 / scene 场景）。
// description 为零成本文字约束（分镜阶段产出）；ref_image 为按需手动生成的参考图。
export interface VisualRef {
  kind: 'character' | 'scene' | string
  name: string
  description: string
  ref_image?: string
}

export interface Series {
  id: string
  name: string
  dynasty: string
  description: string
  // §16：引用的顶层 Voice 条目 ID；创建后锁定，store.UpdateSeries SQL 不写该列。
  voice_id: string
  config: SeriesConfig
  characters: CharacterSetting[]
  created_at: string
  updated_at: string
}

export interface Episode {
  id: string
  series_id: string
  number: number
  title: string
  topic: string
  // 集级附加创作指令（叠加在系列创作设置之上，可空）。
  instruction?: string
  // §17：集级视觉参考（事实源，跨分镜版本共享）。
  refs?: VisualRef[]
  nodes: VersionNode[]
  active_node_id: string
  workdir: string
  created_at: string
  updated_at: string
}

export interface SeriesDetail {
  series: Series
  episodes: Episode[]
}

// ---- AI 分集策划 ----

export interface EpisodeDraft {
  title: string
  topic: string
  summary: string
}

export interface PlanMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface PlanSession {
  series_id: string
  messages: PlanMessage[]
  drafts: EpisodeDraft[]
  characters: CharacterSetting[]
  created_at: string
  updated_at: string
}

export type ActionName =
  | 'story'
  | 'storyboard'
  | 'produce'
  | 'compose'
  | 'run'
  | 'export'
  | 'keyframes'
  | 'episode-refs'

// ---- 平台发布（§19） ----

export type PublishStatus =
  | 'pending'
  | 'uploading'
  | 'uploaded'
  | 'published'
  | 'failed'
  | 'rejected'
  | 'canceled'

export interface PublishJob {
  id: string
  episode_id: string
  series_id: string
  platform: string
  node_id: string
  status: PublishStatus
  video_path: string
  cover_path?: string
  title: string
  description: string
  tags?: string[]
  category?: string
  platform_video_id?: string
  platform_url?: string
  scheduled_at?: string
  attempts: number
  max_retries: number
  error?: string
  created_at: string
  updated_at: string
}

export interface PlatformAccount {
  id: string
  platform: string
  account_name: string
  account_id?: string
  extra?: string
  created_at: string
  updated_at: string
}

export interface PlatformInfo {
  key: string
  name: string
  has_login: boolean
  has_upload: boolean
  has_note: boolean
  has_schedule: boolean
}

export interface AppSettings {
  data_dir: string
  bl_bin: string
  ffmpeg_bin: string
  text_model: string
  video_model: string
  tts_model: string
  tts_voice: string
  image_model: string
  tts_instruction: string
  text_provider: string
  deepseek_api_key?: string
  deepseek_base_url: string
  deepseek_model: string
  bailian_api_key?: string
  bailian_base_url: string
  default_ratio: string
  default_resolution: string
  max_concurrency: number
  max_retries: number
  subtitle_font: string
  sau_bin: string
  python_bin: string
  default_publish_account: string
  bilibili_default_tid: number
}

export interface JobEvent {
  id: string
  action: ActionName | string
  status: 'running' | 'done' | 'failed' | 'canceled'
  error?: string
  started_at: string
  ended_at?: string
}
