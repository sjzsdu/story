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
  tts_voice: string
  tts_instruction: string
  target_platforms?: string[]
  max_concurrency: number
  max_retries: number
}

export interface Series {
  id: string
  name: string
  dynasty: string
  description: string
  config: SeriesConfig
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

export interface JobEvent {
  id: string
  action: ActionName | string
  status: 'running' | 'done' | 'failed'
  error?: string
  started_at: string
  ended_at?: string
}
