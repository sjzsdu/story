import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { PlatformAccount } from '../types'
import { Button, Card, Empty, ErrorBox, Spinner } from '../components/ui'

const PLATFORM_ICONS: Record<string, string> = {
  douyin: '🎵',
  kuaishou: '⚡',
  bilibili: '📺',
  xiaohongshu: '📕',
  tencent: '💬',
  baijiahao: '📰',
  weibo: '🔴',
  hupu: '🏀',
  youtube: '▶️',
  tiktok: '🎶',
}

export default function PlatformsPage() {
  const queryClient = useQueryClient()
  const [selectedPlatform, setSelectedPlatform] = useState<string>('')
  const [errMsg, setErrMsg] = useState('')

  const { data: platforms = [] } = useQuery({
    queryKey: ['platforms'],
    queryFn: api.listPlatforms,
  })

  // 为每个平台查询账号
  const platformAccounts = useQuery({
    queryKey: ['platform-accounts', selectedPlatform],
    queryFn: () => api.listPlatformAccounts(selectedPlatform),
    enabled: !!selectedPlatform,
  })

  // 登录状态检查
  const [checkResults, setCheckResults] = useState<Record<string, boolean>>({})
  const [checking, setChecking] = useState(false)

  const checkAll = async () => {
    if (!accounts) return
    setChecking(true)
    const results: Record<string, boolean> = {}
    for (const acct of accounts) {
      try {
        const res = await api.checkPlatformLogin(selectedPlatform, acct.account_name)
        results[acct.id] = res.valid
      } catch {
        results[acct.id] = false
      }
    }
    setCheckResults(results)
    setChecking(false)
  }

  const deleteMut = useMutation({
    mutationFn: (acct: PlatformAccount) =>
      api.deletePlatformAccount(acct.platform, acct.id),
    onError: (e) => setErrMsg((e as Error).message),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['platform-accounts'] })
    },
  })

  const createMut = useMutation({
    mutationFn: (body: { platform: string; account_name: string }) =>
      api.createPlatformAccount(body.platform, { account_name: body.account_name }),
    onError: (e) => setErrMsg((e as Error).message),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['platform-accounts'] })
    },
  })

  const accounts = platformAccounts.data

  // 自动选中第一个平台
  if (platforms.length > 0 && !selectedPlatform) {
    setSelectedPlatform(platforms[0].key)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="font-display text-2xl tracking-wider text-paper-100">平台账号管理</h1>
        <span className="text-xs text-paper-300/40">登录状态管理 · 多账号支持 · 基于 sau（social-auto-upload）</span>
      </div>

      {errMsg && <ErrorBox>{errMsg}</ErrorBox>}

      {/* 平台选择器 */}
      <div className="flex flex-wrap gap-2">
        {platforms.map((p) => (
          <button
            key={p.key}
            onClick={() => {
              setSelectedPlatform(p.key)
              setCheckResults({})
            }}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors ${
              selectedPlatform === p.key
                ? 'bg-gold-500/20 text-gold-500 border border-gold-500/40'
                : 'bg-ink-800/50 text-paper-300/60 border border-ink-700 hover:border-ink-600 hover:text-paper-300/80'
            }`}
          >
            <span>{PLATFORM_ICONS[p.key] ?? '🔗'}</span>
            <span>{p.name}</span>
          </button>
        ))}
      </div>

      {/* 账号列表 */}
      {selectedPlatform && (
        <Card
          title={`${PLATFORM_ICONS[selectedPlatform] ?? '🔗'} ${platforms.find((p) => p.key === selectedPlatform)?.name ?? selectedPlatform} · 账号`}
          extra={
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                className="px-3 py-1.5 text-xs"
                disabled={checking || !accounts || accounts.length === 0}
                onClick={checkAll}
              >
                {checking ? '检查中…' : '检查全部登录状态'}
              </Button>
              <AddAccountButton
                onAdd={(name) => createMut.mutate({ platform: selectedPlatform, account_name: name })}
                pending={createMut.isPending}
              />
            </div>
          }
        >
          {platformAccounts.isLoading ? (
            <div className="flex justify-center py-8"><Spinner className="w-5 h-5" /></div>
          ) : !accounts || accounts.length === 0 ? (
            <Empty text="暂无账号。点击「添加账号」创建一个，然后用 story platform login 登录扫码。" />
          ) : (
            <div className="space-y-3">
              {accounts.map((acct) => (
                <div
                  key={acct.id}
                  className="flex items-center gap-4 rounded-lg border border-ink-800 bg-ink-950/40 px-4 py-3"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-display text-sm text-paper-100">{acct.account_name}</span>
                      {checkResults[acct.id] !== undefined && (
                        <span className={`text-xs px-1.5 py-0.5 rounded ${checkResults[acct.id] ? 'bg-emerald-500/20 text-emerald-400' : 'bg-red-500/20 text-red-400'}`}>
                          {checkResults[acct.id] ? '已登录' : '未登录/过期'}
                        </span>
                      )}
                    </div>
                    {acct.account_id && (
                      <div className="text-xs text-paper-300/40 mt-0.5">ID: {acct.account_id}</div>
                    )}
                    <div className="text-xs text-paper-300/30 mt-0.5">
                      创建于 {new Date(acct.created_at).toLocaleDateString()}
                    </div>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Button
                      variant="ghost"
                      className="px-2 py-1 text-xs text-red-400/70 hover:text-red-400"
                      disabled={deleteMut.isPending}
                      onClick={() => {
                        if (window.confirm(`确定删除账号「${acct.account_name}」？`)) {
                          deleteMut.mutate(acct)
                        }
                      }}
                    >
                      删除
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}

      {/* 使用说明 */}
      <Card title="使用说明" extra={<span className="text-xs text-paper-300/30">CLI 操作</span>}>
        <div className="text-sm text-paper-300/70 space-y-2 font-body">
          <p><code className="text-gold-500/80 bg-ink-800 px-1.5 py-0.5 rounded text-xs">story platform login {selectedPlatform} --account &lt;name&gt;</code> 扫码登录</p>
          <p><code className="text-gold-500/80 bg-ink-800 px-1.5 py-0.5 rounded text-xs">story platform check {selectedPlatform} --account &lt;name&gt;</code> 检查登录状态</p>
          <p><code className="text-gold-500/80 bg-ink-800 px-1.5 py-0.5 rounded text-xs">story publish run &lt;episode-id&gt; --platform {selectedPlatform} --account &lt;name&gt;</code> 发布</p>
        </div>
      </Card>
    </div>
  )
}

function AddAccountButton({
  onAdd,
  pending,
}: {
  onAdd: (name: string) => void
  pending: boolean
}) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')

  const submit = () => {
    if (!name.trim()) return
    onAdd(name.trim())
    setName('')
    setOpen(false)
  }

  if (!open) {
    return (
      <Button variant="seal" className="px-3 py-1.5 text-xs" onClick={() => setOpen(true)}>
        添加账号
      </Button>
    )
  }

  return (
    <div className="flex items-center gap-2">
      <input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && submit()}
        placeholder="账号名（如 my_douyin）"
        className="px-2.5 py-1 text-sm rounded bg-ink-800 border border-ink-700 text-paper-100 placeholder:text-paper-300/30 w-48"
      />
      <Button variant="seal" className="px-2.5 py-1 text-xs" disabled={pending || !name.trim()} onClick={submit}>
        {pending ? '…' : '确认'}
      </Button>
      <Button variant="ghost" className="px-2 py-1 text-xs" onClick={() => setOpen(false)}>
        取消
      </Button>
    </div>
  )
}
