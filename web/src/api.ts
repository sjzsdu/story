import type {
  ActionName,
  CharacterSetting,
  Episode,
  EpisodeDraft,
  PlanSession,
  Series,
  SeriesDetail,
  JobEvent,
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

  action: (id: string, body: { action: ActionName; index?: number; ratio?: string }) =>
    request<{ job: unknown }>(`/api/episodes/${encodeURIComponent(id)}/actions`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

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
}

/** 把服务器上的绝对文件路径转成受权媒体 URL。 */
export function mediaUrl(episodeId: string, absPath: string): string {
  return `/api/episodes/${encodeURIComponent(episodeId)}/media?path=${encodeURIComponent(absPath)}`
}

/** 系列级媒体 URL（系列视觉参考图等，位于系列项目目录内）。 */
export function seriesMediaUrl(seriesId: string, absPath: string): string {
  return `/api/series/${encodeURIComponent(seriesId)}/media?path=${encodeURIComponent(absPath)}`
}
