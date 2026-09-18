import type {
  ActionName,
  Episode,
  Series,
  SeriesDetail,
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
}

/** 把服务器上的绝对文件路径转成受权媒体 URL。 */
export function mediaUrl(episodeId: string, absPath: string): string {
  return `/api/episodes/${encodeURIComponent(episodeId)}/media?path=${encodeURIComponent(absPath)}`
}
