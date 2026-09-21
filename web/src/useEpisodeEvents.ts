import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import type { Episode, JobEvent } from './types'

export const episodeQueryKey = (id: string) => ['episode', id] as const

/**
 * 订阅某一集的 SSE：
 * - snapshot 事件直接写入 react-query 缓存，页面无需轮询；
 * - job 事件驱动按钮禁用与进度横幅，结束后触发一次最终刷新。
 */
export function useEpisodeEvents(episodeId: string | undefined) {
  const queryClient = useQueryClient()
  const [job, setJob] = useState<JobEvent | null>(null)

  useEffect(() => {
    if (!episodeId) return
    const es = new EventSource(`/api/episodes/${encodeURIComponent(episodeId)}/events`)

    es.addEventListener('snapshot', (e) => {
      const ep = JSON.parse((e as MessageEvent).data) as Episode
      queryClient.setQueryData(episodeQueryKey(episodeId), ep)
    })
    es.addEventListener('job', (e) => {
      const j = JSON.parse((e as MessageEvent).data) as JobEvent
      setJob(j)
      if (j.status === 'done' || j.status === 'failed' || j.status === 'canceled') {
        void queryClient.invalidateQueries({ queryKey: episodeQueryKey(episodeId) })
        void queryClient.invalidateQueries({ queryKey: ['series'] })
      }
    })
    es.onerror = () => {
      // 浏览器会自动重连；任务态在重连后由服务端补推。
    }
    return () => es.close()
  }, [episodeId, queryClient])

  const running = job?.status === 'running'
  return { job, running, triggerRefresh: () => api.getEpisode(episodeId!) }
}
