// 与 Go internal/domain 的 JSON 结构一一对应。

export type StepName = 'generate' | 'pick' | 'storyboard' | 'produce' | 'compose'
export type StepStatus =
  | 'pending'
  | 'running'
  | 'review'
  | 'approved'
  | 'done'
  | 'failed'

export interface StepState {
  status: StepStatus
  attempts: number
  error?: string
  updated_at: string
}

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

export interface PipelineState {
  current: StepName
  steps: Record<StepName, StepState>
  candidates?: StoryCandidate[]
  selected?: number
  story?: StoryCandidate
  storyboard?: Storyboard
  clips?: MediaResult[]
  audios?: MediaResult[]
  outputs?: string[]
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
  voice_profile?: string // 预设 key：wangliqun/kaishu/yizhongtian/shuoshu/cangsang/zhixing
  target_platforms?: string[]
  max_concurrency: number
  max_retries: number
}

// §16：声音顶层实体（与 Series 同级）。series.voice_id 创建后锁定不可改。
export interface Voice {
  id: string
  name: string
  voice: string // 百炼语音 ID，如 longtian_v3
  instruction?: string
  rate?: number
  pitch?: number
  style_note?: string
  is_builtin: boolean
  created_at: string
  updated_at: string
}

// VoiceProfile 旧值对象类型（兼容期保留，server preview/试音仍接收）。
export interface VoiceProfile {
  key?: string
  name: string
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
  state: PipelineState
  refs?: VisualRef[]
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
  | 'candidates'
  | 'pick'
  | 'storyboard'
  | 'produce'
  | 'compose'
  | 'run'
  | 'export'
  | 'keyframes'

export interface JobEvent {
  id: string
  action: ActionName | string
  status: 'running' | 'done' | 'failed'
  error?: string
  started_at: string
  ended_at?: string
}
