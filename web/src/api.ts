import type {
  ActionName,
  CharacterSetting,
  Episode,
  EpisodeDraft,
  PlanSession,
  Series,
  SeriesDetail,
  JobEvent,
  SystemVoice,
  Voice,
} from './types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    const msg = data?.error || `请求失败 (${res.status})`
    throw new Error(msg)
  }
  return data as T
}

export const api = {
  listSeries: () => request<Series[]>('/api/series'),

  createSeries: (body: {
    name: string
    dynasty?: string
    description?: string
    ratio?: string
    resolution?: string
    target_platforms?: string[]
    visual_mode?: 'comic' | 'video'
    // §16：声音条目 ID（推荐）；未传时服务端按平迁规则建/取一个。
    voice_id?: string
  }) => request<Series>('/api/series', { method: 'POST', body: JSON.stringify(body) }),

  getSeries: (id: string) => request<SeriesDetail>(`/api/series/${encodeURIComponent(id)}`),

  deleteSeries: (id: string) =>
    request<{ status: string; id: string }>(`/api/series/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  createEpisode: (seriesId: string, body: { title: string; topic?: string }) =>
    request<Episode>(`/api/series/${encodeURIComponent(seriesId)}/episodes`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  getEpisode: (id: string) => request<Episode>(`/api/episodes/${encodeURIComponent(id)}`),

  deleteEpisode: (id: string) =>
    request<{ status: string; id: string }>(`/api/episodes/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  // action 触发后台流水线动作；from/reroll 为 §17 版本树参数：
  // from 指定起始父节点（留空用集当前活跃节点），reroll 为 true 时开新版本而非复用既有节点。
  action: (
    id: string,
    body: { action: ActionName; from?: string; reroll?: boolean; ratio?: string; scenes?: number[] },
  ) =>
    request<{ job: unknown }>(`/api/episodes/${encodeURIComponent(id)}/actions`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  // activateNode 把某版本节点设为活跃节点（同步、零费用）。
  activateNode: (id: string, nodeId: string) =>
    request<Episode>(
      `/api/episodes/${encodeURIComponent(id)}/nodes/${encodeURIComponent(nodeId)}/activate`,
      { method: 'POST' },
    ),

  // deleteNode 删除节点及其全部后代与媒体文件（同步、不可恢复）。
  deleteNode: (id: string, nodeId: string) =>
    request<Episode>(`/api/episodes/${encodeURIComponent(id)}/nodes/${encodeURIComponent(nodeId)}`, {
      method: 'DELETE',
    }),

  // cancelEpisode 停止正在执行的任务：已完成产物全部保留，之后可再次执行续跑。
  cancelEpisode: (id: string) =>
    request<{ canceled: boolean; job: JobEvent | null }>(
      `/api/episodes/${encodeURIComponent(id)}/cancel`,
      { method: 'POST' },
    ),

  // ---- AI 分集策划 ----
  getPlan: (seriesId: string) =>
    request<PlanSession>(`/api/series/${encodeURIComponent(seriesId)}/plan`),

  planChat: (seriesId: string, message: string) =>
    request<PlanSession>(`/api/series/${encodeURIComponent(seriesId)}/plan/chat`, {
      method: 'POST',
      body: JSON.stringify({ message }),
    }),

  planApply: (seriesId: string, drafts: EpisodeDraft[], characters?: CharacterSetting[]) =>
    request<{ episodes: Episode[] }>(`/api/series/${encodeURIComponent(seriesId)}/plan/apply`, {
      method: 'POST',
      body: JSON.stringify({ drafts, characters }),
    }),

  planReset: (seriesId: string) =>
    request<{ ok: boolean }>(`/api/series/${encodeURIComponent(seriesId)}/plan`, {
      method: 'DELETE',
    }),

  // ---- 系列人物设定与视觉参考图 ----
  updateCharacters: (seriesId: string, characters: CharacterSetting[]) =>
    request<Series>(`/api/series/${encodeURIComponent(seriesId)}/characters`, {
      method: 'PUT',
      body: JSON.stringify({ characters }),
    }),

  generateKeyframes: (seriesId: string, force = false) =>
    request<{ job: JobEvent }>(`/api/series/${encodeURIComponent(seriesId)}/keyframes`, {
      method: 'POST',
      body: JSON.stringify({ force }),
    }),

  // ---- 本集视觉参考图（人物/场景，手动按张计费生成） ----
  generateEpisodeRefs: (episodeId: string, force = false) =>
    request<{ job: JobEvent }>(`/api/episodes/${encodeURIComponent(episodeId)}/refs`, {
      method: 'POST',
      body: JSON.stringify({ force }),
    }),

  // ---- 声音（顶层实体，§16） ----
  // GET /api/voices 返回 []Voice；旧 listVoices 返回 VoiceProfile[] 已废弃。
  listVoices: () => request<Voice[]>('/api/voices'),

  getVoice: (id: string) =>
    request<Voice>(`/api/voices/${encodeURIComponent(id)}`),

  createVoice: (body: {
    name: string
    provider?: string
    voice: string
    model?: string
    instruction?: string
    rate?: number
    pitch?: number
    style_note?: string
  }) => request<Voice>('/api/voices', {
    method: 'POST',
    body: JSON.stringify(body),
  }),

  updateVoice: (id: string, body: {
    name?: string
    provider?: string
    voice?: string
    model?: string
    instruction?: string
    rate?: number
    pitch?: number
    style_note?: string
  }) => request<Voice>(`/api/voices/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  }),

  // 造声（§16）：声音设计（文字描述生成全新音色）与声音复刻（上传音频克隆音色）。
  // 成功后返回新建的声音条目与试听音频路径（按新建音色个数计费）。
  designVoice: (body: {
    name: string
    provider?: string
    prompt: string
    preview_text?: string
    target_model?: string
    language_hints?: string[]
    style_note?: string
  }) => request<{ voice: Voice; preview_audio_path: string }>('/api/voices/design', {
    method: 'POST',
    body: JSON.stringify(body),
  }),

  cloneVoice: (body: {
    name: string
    provider?: string
    audio_path?: string
    audio_url?: string
    target_model?: string
    language_hints?: string[]
    max_prompt_audio_length?: number
    enable_preprocess?: boolean
    style_note?: string
  }) => request<{ voice: Voice; preview_audio_path: string }>('/api/voices/clone', {
    method: 'POST',
    body: JSON.stringify(body),
  }),

  // uploadVoiceSample 上传参考音频（浏览器选择的文件 / 现场录音）到服务器。
  // 服务端用 ffmpeg 统一归一化为 16kHz 单声道 wav 并返回服务器路径与时长，
  // 前端再拿该路径调 cloneVoice（复刻只在这一步计费）。
  // 注意：必须用 headers: {} 覆盖 request 里默认的 JSON Content-Type，
  // 否则 multipart boundary 会丢失、后端解析失败。
  uploadVoiceSample: (blob: Blob, filename: string) => {
    const fd = new FormData()
    fd.append('file', blob, filename)
    return request<{ path: string; duration_sec: number }>('/api/voices/audio', {
      method: 'POST',
      body: fd,
      headers: {},
    })
  },

  // listSystemVoices 浏览某 TTS 供应商的系统音色（只取元数据，不合成、不产生费用）。
  listSystemVoices: (provider: string, model?: string) => {
    const q = model ? `?model=${encodeURIComponent(model)}` : ''
    return request<SystemVoice[]>(
      `/api/voice-providers/${encodeURIComponent(provider)}/voices${q}`,
    )
  },

  deleteVoice: (id: string) =>
    request<{ status: string; id: string }>(
      `/api/voices/${encodeURIComponent(id)}`,
      { method: 'DELETE' },
    ),

  // previewVoice 三种入口（任选其一）：
  //   voice_id：按顶层 Voice 条目解析（推荐）
  //   profile：旧预设 key（回退兼容）
  //   voice + rate + pitch + instruction：裸自定义参数
  previewVoice: (body: {
    voice_id?: string
    profile?: string
    voice?: string
    rate?: number
    pitch?: number
    instruction?: string
    text?: string
  }) => request<{ path: string }>('/api/voices/preview', {
    method: 'POST',
    body: JSON.stringify(body),
  }),
}

/** 把服务器上的绝对文件路径转成受权媒体 URL。 */
export function mediaUrl(episodeId: string, absPath: string): string {
  return `/api/episodes/${encodeURIComponent(episodeId)}/media?path=${encodeURIComponent(absPath)}`
}

/** 系列级媒体 URL（系列视觉参考图等，位于系列项目目录内）。 */
export function seriesMediaUrl(seriesId: string, absPath: string): string {
  return `/api/series/${encodeURIComponent(seriesId)}/media?path=${encodeURIComponent(absPath)}`
}
