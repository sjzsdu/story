import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import type { JobEvent, SeriesDetail } from './types'

export const seriesQueryKey = (id: string) => ['series', id] as const

/**
 * 订阅系列级 SSE：
 * - snapshot（系列+集列表）直接写入 react-query 缓存；
 * - job 事件驱动视觉参考图等系列级后台动作的按钮状态，结束后触发一次最终刷新。
 */
export function useSeriesEvents(seriesId: string | undefined) {
  const queryClient = useQueryClient()
  const [job, setJob] = useState<JobEvent | null>(null)

  useEffect(() => {
    if (!seriesId) return
    const es = new EventSource(`/api/series/${encodeURIComponent(seriesId)}/events`)

    es.addEventListener('snapshot', (e) => {
      const snap = JSON.parse((e as MessageEvent).data) as SeriesDetail
      queryClient.setQueryData(seriesQueryKey(seriesId), snap)
    })
    es.addEventListener('job', (e) => {
      const j = JSON.parse((e as MessageEvent).data) as JobEvent
      setJob(j)
      if (j.status === 'done' || j.status === 'failed') {
        void queryClient.invalidateQueries({ queryKey: seriesQueryKey(seriesId) })
      }
    })
    es.onerror = () => {
      // 浏览器会自动重连；任务态在重连后由服务端补推。
    }
    return () => es.close()
  }, [seriesId, queryClient])

  const running = job?.status === 'running'
  return { job, running }
}
