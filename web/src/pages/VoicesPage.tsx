import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { Voice } from '../types'
import { Button, Card, Empty, ErrorBox, Field, Modal, Select, Spinner, TextInput } from '../components/ui'

// TTS 供应商选项。一条声音条目只属于一个供应商（§16，2026-09-20）；
// 当前系统只接入百炼，未来接入新供应商时在此追加，VoiceFormModal 与
// 浏览音色库自动按所选供应商取数。
const PROVIDER_OPTIONS = [
	{ id: 'bailian', label: '阿里云百炼 CosyVoice' },
]

export default function VoicesPage() {
	const { data: voices, isLoading, error } = useQuery({ queryKey: ['voices'], queryFn: api.listVoices })
	// createInitial：新建 Modal 的预填值——复制声音/试音后保存都走它。
	const [showCreate, setShowCreate] = useState(false)
	const [createInitial, setCreateInitial] = useState<VoiceFormInitial | undefined>(undefined)
	const [editing, setEditing] = useState<Voice | null>(null)
	// 造声 Modal：design=声音设计（文字描述）/ clone=声音复刻（上传音频）。
	const [buildKind, setBuildKind] = useState<'design' | 'clone' | null>(null)

	const openCreate = (initial?: VoiceFormInitial) => {
		setCreateInitial(initial)
		setShowCreate(true)
	}

	return (
		<div className="space-y-6">
			<div className="flex items-end justify-between gap-4 flex-wrap">
				<div>
					<h1 className="font-display text-2xl tracking-wider">声音库</h1>
					<p className="mt-1 text-sm text-paper-300/50">
						与系列同级的顶层实体。新建系列时选定一个声音，之后锁定不可改；编辑条目本身会同步影响所有引用它的系列
					</p>
				</div>
				<div className="flex gap-2 flex-wrap">
					<Button variant="seal" onClick={() => openCreate(undefined)}>
						＋ 新建声音
					</Button>
					<Button variant="outline" onClick={() => setBuildKind('design')}>
						声音设计
					</Button>
					<Button variant="outline" onClick={() => setBuildKind('clone')}>
						声音复刻
					</Button>
				</div>
			</div>

			{error && <ErrorBox>{(error as Error).message}</ErrorBox>}

			{isLoading ? (
				<div className="flex justify-center py-16">
					<Spinner className="w-6 h-6" />
				</div>
			) : !voices || voices.length === 0 ? (
				<Card>
					<Empty text="还没有声音条目（内置条目启动时会自动 seed，如缺失请重启服务）" />
				</Card>
			) : (
				<div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-4">
					{voices.map((v) => (
						<VoiceCard
							key={v.id}
							voice={v}
							onEdit={() => setEditing(v)}
							onDuplicate={() => openCreate(voiceToInitial(v, true))}
						/>
					))}
				</div>
			)}

			{showCreate && (
				<VoiceFormModal
					mode="create"
					initial={createInitial}
					onClose={() => setShowCreate(false)}
				/>
			)}
			{editing && (
				<VoiceFormModal
					mode="edit"
					voiceId={editing.id}
					builtin={editing.is_builtin}
					initial={voiceToInitial(editing)}
					onClose={() => setEditing(null)}
				/>
			)}
			{buildKind && (
				<BuildVoiceModal
					kind={buildKind}
					onClose={() => setBuildKind(null)}
				/>
			)}
		</div>
	)
}

function VoiceCard({
	voice: v,
	onEdit,
	onDuplicate,
}: {
	voice: Voice
	onEdit: () => void
	onDuplicate: () => void
}) {
	const qc = useQueryClient()
	const [previewPath, setPreviewPath] = useState<string | null>(null)
	const [err, setErr] = useState('')

	const previewMut = useMutation({
		mutationFn: () => api.previewVoice({ voice_id: v.id }),
		onSuccess: (data) => {
			setErr('')
			setPreviewPath(data.path)
		},
		onError: (e) => setErr((e as Error).message),
	})

	const deleteMut = useMutation({
		mutationFn: () => api.deleteVoice(v.id),
		onSuccess: () => {
			setErr('')
			void qc.invalidateQueries({ queryKey: ['voices'] })
		},
		onError: (e) => setErr((e as Error).message),
	})

	return (
		<div className="rounded-xl border border-ink-800 bg-ink-950/60 p-5 flex flex-col">
			<div className="flex items-start justify-between gap-2 mb-2">
				<div className="font-display text-gold-500">{v.name}</div>
				{v.is_builtin && (
					<span className="text-[11px] rounded-full px-2 py-0.5 border text-emerald-300/70 border-emerald-400/30 bg-emerald-400/10">
						内置
					</span>
				)}
			</div>

			<p className="text-xs text-paper-300/55 leading-relaxed mb-3">{v.style_note || '（无风格说明）'}</p>

			<dl className="text-xs text-paper-300/50 space-y-1 mb-3">
				<div className="flex gap-2">
					<dt className="w-14 shrink-0 text-paper-300/35">供应商</dt>
					<dd>{v.provider || 'bailian'}</dd>
				</div>
				<div className="flex gap-2">
					<dt className="w-14 shrink-0 text-paper-300/35">音色</dt>
					<dd className="truncate">{v.voice}</dd>
				</div>
				{v.model && (
					<div className="flex gap-2">
						<dt className="w-14 shrink-0 text-paper-300/35">模型</dt>
						<dd className="truncate" title={v.model}>{v.model}</dd>
					</div>
				)}
				{(v.rate ?? 0) > 0 && (
					<div className="flex gap-2">
						<dt className="w-14 shrink-0 text-paper-300/35">语速</dt>
						<dd>{(v.rate ?? 0).toFixed(2)}</dd>
					</div>
				)}
				{(v.pitch ?? 0) > 0 && (v.pitch ?? 0) !== 1 && (
					<div className="flex gap-2">
						<dt className="w-14 shrink-0 text-paper-300/35">音高</dt>
						<dd>{(v.pitch ?? 0).toFixed(2)}</dd>
					</div>
				)}
				{v.instruction && (
					<div className="flex gap-2">
						<dt className="w-14 shrink-0 text-paper-300/35">指令</dt>
						<dd className="truncate" title={v.instruction}>{v.instruction}</dd>
					</div>
				)}
			</dl>

			<div className="mt-auto flex items-center gap-2 pt-2 border-t border-ink-800">
				<button
					className="text-xs text-sky-400/70 hover:text-sky-400 disabled:opacity-30"
					disabled={previewMut.isPending}
					onClick={() => previewMut.mutate()}
				>
					{previewMut.isPending ? '合成中…' : '试听'}
				</button>
				<button
					className="text-xs text-paper-300/50 hover:text-paper-100"
					onClick={onDuplicate}
				>
					复制
				</button>
				<button
					className="ml-auto text-xs text-paper-300/50 hover:text-paper-100"
					onClick={onEdit}
				>
					编辑
				</button>
				{!v.is_builtin && (
					<button
						className="text-xs text-seal-500/70 hover:text-seal-500 disabled:opacity-30"
						disabled={deleteMut.isPending}
						onClick={() => {
							if (window.confirm(`确定删除声音「${v.name}」？若被系列引用会被拒绝。`)) {
								deleteMut.mutate()
							}
						}}
					>
						{deleteMut.isPending ? '删除中…' : '删除'}
					</button>
				)}
			</div>

			{err && <div className="mt-2"><ErrorBox>{err}</ErrorBox></div>}

			{previewPath && (
				<div className="mt-3 rounded-lg border border-ink-700 bg-ink-950/40 p-2">
					<div className="flex items-center justify-between mb-1">
						<span className="text-xs text-paper-300/50">试听样音</span>
						<button
							className="text-xs text-paper-300/30 hover:text-paper-300/60"
							onClick={() => setPreviewPath(null)}
						>
							关闭
						</button>
					</div>
					<audio
						controls
						autoPlay
						className="w-full h-9"
						src={`/api/voices/preview?path=${encodeURIComponent(previewPath)}`}
					/>
				</div>
			)}
		</div>
	)
}

// ---- 表单 ----

// VoiceFormInitial 表单初值（创建/编辑/复制共用）。
interface VoiceFormInitial {
	name: string
	provider: string
	voice: string
	model: string
	instruction: string
	rate: number
	pitch: number
	styleNote: string
}

function voiceToInitial(v: Voice, isCopy = false): VoiceFormInitial {
	return {
		name: isCopy ? `${v.name} 副本` : v.name,
		provider: v.provider || 'bailian',
		voice: v.voice,
		model: v.model || '',
		instruction: v.instruction || '',
		rate: v.rate || 0,
		pitch: v.pitch || 0,
		styleNote: v.style_note || '',
	}
}

function VoiceFormModal({
	mode,
	voiceId,
	builtin = false,
	initial,
	onClose,
}: {
	mode: 'create' | 'edit'
	voiceId?: string
	builtin?: boolean
	initial?: VoiceFormInitial
	onClose: () => void
}) {
	const qc = useQueryClient()
	const [name, setName] = useState(initial?.name ?? '')
	const [provider, setProvider] = useState(initial?.provider ?? 'bailian')
	const [voice, setVoice] = useState(initial?.voice ?? 'longtian_v3')
	const [model, setModel] = useState(initial?.model ?? '')
	const [instruction, setInstruction] = useState(initial?.instruction ?? '')
	const [rate, setRate] = useState(initial?.rate ?? 0)
	const [pitch, setPitch] = useState(initial?.pitch ?? 0)
	const [styleNote, setStyleNote] = useState(initial?.styleNote ?? '')
	const [err, setErr] = useState('')

	// 浏览音色库（内嵌展开，不嵌套 Modal）。
	const [browseOpen, setBrowseOpen] = useState(false)
	const [keyword, setKeyword] = useState('')
	const sysQuery = useQuery({
		queryKey: ['system-voices', provider],
		queryFn: () => api.listSystemVoices(provider),
		enabled: browseOpen,
		staleTime: 5 * 60_000,
	})
	const filtered = (sysQuery.data ?? []).filter((sv) => {
		const k = keyword.trim().toLowerCase()
		if (!k) return true
		return `${sv.id} ${sv.name} ${sv.description}`.toLowerCase().includes(k)
	})

	// 试听：按当前表单参数实时合成，满意后点保存（"先试音再保存"）。
	const [previewPath, setPreviewPath] = useState<string | null>(null)
	const trialMut = useMutation({
		mutationFn: () => api.previewVoice({ voice, rate, pitch, instruction }),
		onSuccess: (d) => {
			setErr('')
			setPreviewPath(d.path)
		},
		onError: (e) => setErr((e as Error).message),
	})

	const saveMut = useMutation({
		mutationFn: () => {
			if (mode === 'create') {
				return api.createVoice({
					name: name.trim(),
					provider,
					voice: voice.trim(),
					model: model.trim(),
					instruction: instruction.trim(),
					rate,
					pitch,
					style_note: styleNote.trim(),
				})
			}
			return api.updateVoice(voiceId!, {
				name: name.trim(),
				provider,
				voice: voice.trim(),
				model: model.trim(),
				instruction: instruction.trim(),
				rate,
				pitch,
				style_note: styleNote.trim(),
			})
		},
		onSuccess: () => {
			void qc.invalidateQueries({ queryKey: ['voices'] })
			void qc.invalidateQueries({ queryKey: ['series'] })
			onClose()
		},
		onError: (e) => setErr((e as Error).message),
	})

	const title =
		mode === 'create'
			? '新建声音'
			: `编辑声音 · ${initial?.name}${builtin ? '（内置）' : ''}`

	return (
		<Modal open onClose={onClose} title={title} wide>
			<form
				className="grid sm:grid-cols-2 gap-4"
				onSubmit={(e) => {
					e.preventDefault()
					setErr('')
					saveMut.mutate()
				}}
			>
				<Field label="名称（必填）">
					<TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder="如：磁性理智男" autoFocus />
				</Field>
				<Field label="TTS 供应商">
					<Select value={provider} onChange={(e) => setProvider(e.target.value)}>
						{PROVIDER_OPTIONS.map((p) => (
							<option key={p.id} value={p.id}>{p.label}</option>
						))}
					</Select>
				</Field>

				<Field label="音色 ID（必填）">
					<div className="flex gap-2">
						<TextInput value={voice} onChange={(e) => setVoice(e.target.value)} placeholder="如 longtian_v3" />
						<Button
							type="button"
							variant="outline"
							onClick={() => setBrowseOpen((v) => !v)}
						>
							{browseOpen ? '收起' : '浏览音色库'}
						</Button>
					</div>

					{browseOpen && (
						<div className="mt-2 rounded-lg border border-ink-700 bg-ink-950/50 p-2">
							<TextInput
								value={keyword}
								onChange={(e) => setKeyword(e.target.value)}
								placeholder="搜索音色 ID / 名称 / 特质…"
							/>
							{sysQuery.isLoading ? (
								<div className="flex justify-center py-4">
									<Spinner className="w-4 h-4" />
								</div>
							) : sysQuery.isError ? (
								<p className="mt-2 text-xs text-seal-500/80 py-2">
									{(sysQuery.error as Error).message}
								</p>
							) : (
								<ul className="mt-2 max-h-48 overflow-y-auto divide-y divide-ink-800">
									{filtered.map((sv) => (
										<li key={sv.id}>
											<button
												type="button"
												className="w-full flex items-center justify-between gap-2 px-1 py-1.5 text-left hover:bg-ink-800/50 rounded"
												onClick={() => {
													setVoice(sv.id)
													// 名称留空时顺手用音色中文名，用户可再改。
													if (!name.trim()) {
														setName(sv.name)
													}
													setKeyword('')
												}}
											>
												<span className="text-xs text-paper-200/80">
													{sv.name}
													<span className="ml-1.5 text-paper-300/40">{sv.id}</span>
												</span>
												<span className="text-[11px] text-paper-300/45 shrink-0">
													{sv.description}
												</span>
											</button>
										</li>
									))}
									{filtered.length === 0 && (
										<li className="text-xs text-paper-300/40 py-3 text-center">无匹配音色</li>
									)}
								</ul>
							)}
						</div>
					)}
				</Field>

				<Field label="驱动模型（造声音色必填，普通音色留空）">
					<TextInput
						value={model}
						onChange={(e) => setModel(e.target.value)}
						placeholder="如 cosyvoice-v3-flash（造声时必须与造声模型一致）"
					/>
				</Field>
				<Field label="风格指令（可选，部分音色不支持）">
					<TextInput
						value={instruction}
						onChange={(e) => setInstruction(e.target.value)}
						placeholder="如：沉稳、有书卷气、节奏从容"
					/>
				</Field>
				<Field label="风格说明（仅展示用）">
					<TextInput
						value={styleNote}
						onChange={(e) => setStyleNote(e.target.value)}
						placeholder="如：磁性理智男，语速沉稳"
					/>
				</Field>
				<Field label="语速（0.5-2.0，0=默认）">
					<input
						type="range"
						min="0"
						max="2"
						step="0.05"
						value={rate}
						onChange={(e) => setRate(parseFloat(e.target.value))}
						className="w-full"
					/>
					<span className="text-xs text-paper-300/50">{rate.toFixed(2)}</span>
				</Field>
				<Field label="音高（0.5-2.0，0=默认）">
					<input
						type="range"
						min="0"
						max="2"
						step="0.05"
						value={pitch}
						onChange={(e) => setPitch(parseFloat(e.target.value))}
						className="w-full"
					/>
					<span className="text-xs text-paper-300/50">{pitch.toFixed(2)}</span>
				</Field>

				{previewPath && (
					<div className="sm:col-span-2 rounded-lg border border-ink-700 bg-ink-950/40 p-2">
						<div className="flex items-center justify-between mb-1">
							<span className="text-xs text-paper-300/50">试听当前参数</span>
							<button
								type="button"
								className="text-xs text-paper-300/30 hover:text-paper-300/60"
								onClick={() => setPreviewPath(null)}
							>
								关闭
							</button>
						</div>
						<audio
							controls
							autoPlay
							className="w-full h-9"
							src={`/api/voices/preview?path=${encodeURIComponent(previewPath)}`}
						/>
					</div>
				)}

				{err && (
					<div className="sm:col-span-2">
						<ErrorBox>{err}</ErrorBox>
					</div>
				)}

				<div className="sm:col-span-2 flex gap-3 flex-wrap">
					<Button type="submit" variant="seal" disabled={saveMut.isPending || !name.trim() || !voice.trim()}>
						{saveMut.isPending && <Spinner />}
						{mode === 'create' ? '创建' : '保存'}
					</Button>
					<Button
						type="button"
						variant="outline"
						disabled={trialMut.isPending || !voice.trim()}
						onClick={() => trialMut.mutate()}
					>
						{trialMut.isPending && <Spinner />}
						{trialMut.isPending ? '合成中…' : '先试听'}
					</Button>
					<Button type="button" variant="ghost" onClick={onClose}>
						取消
					</Button>
				</div>
			</form>
		</Modal>
	)
}

// ---- 造声（声音设计 / 声音复刻，§16）----

// 造声音色必须用造声时的驱动模型合成，否则合成会失败；此处默认 cosyvoice-v3-flash。
const BUILD_MODEL_DEFAULT = 'cosyvoice-v3-flash'

const TEXTAREA_CLS =
	'w-full rounded-lg border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-paper-100 placeholder:text-paper-300/30 outline-none focus:border-gold-500/60 resize-y'

// 复刻参考音频的来源。三者平级：上传文件 / 现场录音 / 公网 URL。
type SampleTab = 'upload' | 'record' | 'url'

const SAMPLE_TABS: { id: SampleTab; label: string }[] = [
	{ id: 'upload', label: '上传音频文件' },
	{ id: 'record', label: '现场录音' },
	{ id: 'url', label: '音频 URL' },
]

// 供应商要求参考音频 3-30 秒：录音到 30 秒自动停止，前端也提前拦一次过短。
const CLONE_MAX_SEC = 30
const CLONE_MIN_SEC = 3

// 推荐朗读文本：含叙述句与设问句，起伏完整，便于试出音色全貌；页面内可编辑。
// 正常语速约 25 秒，正好落在供应商要求的 3-30 秒区间内。
const CLONE_SAMPLE_TEXT =
	'话说天下大势，分久必合，合久必分。夜半，城头的号角忽然响起——你可知道，是谁在等这一场大雨？三十年前的那个春天，他也曾站在这里，望着滚滚东去的江水，久久没有作声。风从旷野吹来，卷起昨日的烟尘，把所有的誓言，都带走了。'

// pickAudioMime 探测浏览器支持的录音容器：优先 webm/opus（Chrome/Firefox），
// 回退 mp4（Safari）。服务端会用 ffmpeg 统一归一化，两者都能收。
function pickAudioMime(): string | undefined {
	if (typeof MediaRecorder === 'undefined') return undefined
	const candidates = ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4', 'audio/ogg;codecs=opus']
	return candidates.find((t) => MediaRecorder.isTypeSupported(t))
}

// blobFilename 按录音容器给一个合理的文件名后缀（仅作排查便利，服务端会截断校验）。
function blobFilename(blob: Blob): string {
	const t = blob.type || ''
	const ext = t.includes('mp4') ? 'mp4' : t.includes('ogg') ? 'ogg' : t.includes('wav') ? 'wav' : 'webm'
	return `recording-${Date.now()}.${ext}`
}

function fmtSec(sec: number): string {
	return sec.toFixed(1)
}

function BuildVoiceModal({
	kind,
	onClose,
}: {
	kind: 'design' | 'clone'
	onClose: () => void
}) {
	const qc = useQueryClient()
	const isDesign = kind === 'design'
	const [provider, setProvider] = useState('bailian')
	const [name, setName] = useState('')
	const [prompt, setPrompt] = useState('')
	const [previewText, setPreviewText] = useState('')
	// 复刻参考音频：三 Tab 里选一种来源。
	const [sampleTab, setSampleTab] = useState<SampleTab>('upload')
	const [file, setFile] = useState<File | null>(null)
	const [readText, setReadText] = useState(CLONE_SAMPLE_TEXT)
	const [recording, setRecording] = useState(false)
	const [elapsed, setElapsed] = useState(0)
	const [clip, setClip] = useState<Blob | null>(null)
	const [clipURL, setClipURL] = useState('')
	const [audioURL, setAudioURL] = useState('')
	const [model, setModel] = useState('')
	const [maxAudio, setMaxAudio] = useState(0)
	const [preprocess, setPreprocess] = useState(false)
	const [err, setErr] = useState('')
	// 复刻是两段式：先把音频送服务器归一化（uploading），再提交克隆（building）。
	const [stage, setStage] = useState<'idle' | 'uploading' | 'building'>('idle')
	const [result, setResult] = useState<{ voice: Voice; previewPath: string } | null>(null)

	// 录音相关引用：MediaRecorder/流、计时器、起始时刻、当前试听 objectURL。
	const mediaRef = useRef<{ rec: MediaRecorder; stream: MediaStream } | null>(null)
	const tickRef = useRef<number | null>(null)
	const startAtRef = useRef(0)
	const clipURLRef = useRef('')

	const clearTick = useCallback(() => {
		if (tickRef.current !== null) {
			window.clearInterval(tickRef.current)
			tickRef.current = null
		}
	}, [])

	// setClipBlob 同步维护试听 URL，替换时释放上一个 objectURL，避免泄漏。
	const setClipBlob = useCallback((blob: Blob | null) => {
		if (clipURLRef.current) {
			URL.revokeObjectURL(clipURLRef.current)
			clipURLRef.current = ''
		}
		setClip(blob)
		const url = blob ? URL.createObjectURL(blob) : ''
		clipURLRef.current = url
		setClipURL(url)
	}, [])

	const stopRecording = useCallback(() => {
		clearTick()
		setRecording(false)
		const m = mediaRef.current
		if (m && m.rec.state !== 'inactive') m.rec.stop()
	}, [clearTick])

	// 卸载/关闭弹窗时必须释放麦克风与计时器，否则浏览器会一直显示录音中。
	useEffect(
		() => () => {
			clearTick()
			const m = mediaRef.current
			if (m) {
				if (m.rec.state !== 'inactive') m.rec.stop()
				m.stream.getTracks().forEach((t) => t.stop())
			}
			if (clipURLRef.current) URL.revokeObjectURL(clipURLRef.current)
		},
		[clearTick],
	)

	const startRecording = async () => {
		setErr('')
		if (typeof MediaRecorder === 'undefined' || !navigator.mediaDevices?.getUserMedia) {
			setErr('当前浏览器不支持页面录音，请改用「上传音频文件」或「音频 URL」')
			return
		}
		try {
			const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
			const mime = pickAudioMime()
			const rec = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined)
			const chunks: Blob[] = []
			rec.ondataavailable = (e) => {
				if (e.data.size > 0) chunks.push(e.data)
			}
			rec.onstop = () => {
				stream.getTracks().forEach((t) => t.stop())
				setClipBlob(new Blob(chunks, { type: rec.mimeType || 'audio/webm' }))
			}
			mediaRef.current = { rec, stream }
			startAtRef.current = Date.now()
			setClipBlob(null)
			setElapsed(0)
			rec.start()
			setRecording(true)
			// 200ms 刷新计时足够跟手；到上限自动停止，避免录出超长音频被供应商拒绝。
			tickRef.current = window.setInterval(() => {
				const sec = (Date.now() - startAtRef.current) / 1000
				setElapsed(sec)
				if (sec >= CLONE_MAX_SEC) stopRecording()
			}, 200)
		} catch (e) {
			setErr(`无法访问麦克风：${(e as Error).message}。请检查浏览器权限（需 https 或 localhost），或改用「上传音频文件」`)
		}
	}

	const buildMut = useMutation({
		mutationFn: async () => {
			const targetModel = model.trim() || BUILD_MODEL_DEFAULT
			if (isDesign) {
				return api.designVoice({
					name: name.trim(),
					provider,
					prompt: prompt.trim(),
					preview_text: previewText.trim() || undefined,
					target_model: targetModel,
				})
			}

			// 复刻：URL 直接提交；上传/录音先送服务器归一化拿到路径。
			let audioPath = ''
			if (sampleTab !== 'url') {
				const blob: Blob | null = sampleTab === 'record' ? clip : file
				if (!blob) {
					throw new Error(sampleTab === 'record' ? '请先录制一段参考音频' : '请先选择参考音频文件')
				}
				setStage('uploading')
				const up = await api.uploadVoiceSample(
					blob,
					sampleTab === 'record' ? blobFilename(blob) : (file?.name ?? 'sample.webm'),
				)
				if (up.duration_sec < CLONE_MIN_SEC) {
					throw new Error(
						`参考音频过短（${fmtSec(up.duration_sec)} 秒），请提供 ${CLONE_MIN_SEC}-${CLONE_MAX_SEC} 秒的清晰人声`,
					)
				}
				audioPath = up.path
			}

			setStage('building')
			return api.cloneVoice({
				name: name.trim(),
				provider,
				audio_path: audioPath || undefined,
				audio_url: sampleTab === 'url' ? audioURL.trim() || undefined : undefined,
				target_model: targetModel,
				max_prompt_audio_length: maxAudio > 0 ? maxAudio : undefined,
				enable_preprocess: preprocess,
			})
		},
		onSuccess: (d) => {
			setErr('')
			setResult({ voice: d.voice, previewPath: d.preview_audio_path })
			void qc.invalidateQueries({ queryKey: ['voices'] })
		},
		onError: (e) => setErr((e as Error).message),
		onSettled: () => setStage('idle'),
	})

	const canSubmit = isDesign
		? !!name.trim() && !!prompt.trim()
		: !!name.trim() &&
			(sampleTab === 'url' ? !!audioURL.trim() : sampleTab === 'record' ? !!clip : !!file)

	return (
		<Modal open onClose={onClose} title={isDesign ? '声音设计 · 文字描述生成音色' : '声音复刻 · 提供参考音频克隆音色'} wide>
			<form
				className="grid sm:grid-cols-2 gap-4"
				onSubmit={(e) => {
					e.preventDefault()
					setErr('')
					buildMut.mutate()
				}}
			>
				<div className="sm:col-span-2 text-xs text-paper-300/50 leading-relaxed">
					{isDesign
						? '用文字描述目标音色，百炼生成一个全新音色。按新建音色个数计费，请谨慎提交。'
						: '提供一段 3-30 秒的参考音频（上传文件 / 现场录音 / 音频 URL），百炼克隆出音色。禁止克隆他人真人声音（声音权法律风险）。按新建音色个数计费。'}
					<span className="ml-1 text-paper-300/40">造出的音色需用造声时的驱动模型合成，条目已自动记录模型。</span>
				</div>

				<Field label="名称（必填）">
					<TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder="如：说书人·沉稳男声" autoFocus />
				</Field>
				<Field label="TTS 供应商（造声能力按供应商分发）">
					<Select value={provider} onChange={(e) => setProvider(e.target.value)}>
						{PROVIDER_OPTIONS.map((p) => (
							<option key={p.id} value={p.id}>{p.label}</option>
						))}
					</Select>
				</Field>
				<Field label="驱动模型（留空默认 cosyvoice-v3-flash）">
					<TextInput value={model} onChange={(e) => setModel(e.target.value)} placeholder={BUILD_MODEL_DEFAULT} />
				</Field>

				{isDesign ? (
					<>
						<div className="sm:col-span-2">
							<Field label="声音描述（必填）">
								<textarea
									className={TEXTAREA_CLS}
									rows={3}
									value={prompt}
									onChange={(e) => setPrompt(e.target.value)}
									placeholder="如：沉稳的中年男声，书卷气，语速从容，适合讲述历史故事"
								/>
							</Field>
						</div>
						<div className="sm:col-span-2">
							<Field label="试听文本（留空用默认）">
								<TextInput
									value={previewText}
									onChange={(e) => setPreviewText(e.target.value)}
									placeholder="留空用「话说天下大势，分久必合，合久必分。」"
								/>
							</Field>
						</div>
					</>
				) : (
					<>
						{/* 参考音频来源：上传文件 / 现场录音 / 公网 URL，三选一。 */}
						<div className="sm:col-span-2 flex gap-2 flex-wrap">
							{SAMPLE_TABS.map((t) => (
								<button
									key={t.id}
									type="button"
									onClick={() => {
										setSampleTab(t.id)
										setErr('')
									}}
									className={`rounded-lg border px-3.5 py-2 text-sm transition-colors ${
										sampleTab === t.id
											? 'border-gold-500/60 bg-gold-500/10 text-gold-500'
											: 'border-ink-700 text-paper-300/70 hover:border-ink-600 hover:text-paper-100'
									}`}
								>
									{t.label}
								</button>
							))}
						</div>

						{sampleTab === 'upload' && (
							<div className="sm:col-span-2">
								<Field label={`参考音频文件（mp3 / wav / m4a / webm 等，${CLONE_MIN_SEC}-${CLONE_MAX_SEC} 秒）`}>
									<input
										type="file"
										accept="audio/*"
										onChange={(e) => {
											setFile(e.target.files?.[0] ?? null)
											setErr('')
										}}
										className="w-full text-sm text-paper-300/70 file:mr-3 file:rounded-lg file:border file:border-ink-600 file:bg-ink-800 file:px-3 file:py-1.5 file:text-sm file:text-paper-100 hover:file:border-gold-500/60"
									/>
								</Field>
								<p className="mt-1.5 text-xs text-paper-300/50">
									{file
										? `已选：${file.name}（${(file.size / 1024).toFixed(0)} KB）· 提交时上传服务器并自动归一化为 16kHz 单声道`
										: '文件只在提交时上传到服务器，不会随表单预传。'}
								</p>
							</div>
						)}

						{sampleTab === 'record' && (
							<>
								<div className="sm:col-span-2">
									<Field label="照着念这段文字（可编辑，正常语速约 25 秒）">
										<textarea
											className={TEXTAREA_CLS}
											rows={4}
											value={readText}
											onChange={(e) => setReadText(e.target.value)}
											disabled={recording}
										/>
									</Field>
								</div>
								<div className="sm:col-span-2 flex items-center gap-3 flex-wrap">
									{recording ? (
										<Button type="button" variant="seal" onClick={stopRecording}>
											■ 停止录音（{fmtSec(elapsed)}s / {CLONE_MAX_SEC}s）
										</Button>
									) : (
										<Button type="button" variant="primary" onClick={startRecording}>
											● {clip ? '重录' : '开始录音'}
										</Button>
									)}
									{clip && <audio controls src={clipURL} className="h-9 flex-1 min-w-[12rem]" />}
									{!clip && !recording && (
										<span className="text-xs text-paper-300/50">
											点「开始录音」时浏览器会请求麦克风权限；录满 {CLONE_MAX_SEC} 秒自动停止。
										</span>
									)}
								</div>
							</>
						)}

						{sampleTab === 'url' && (
							<div className="sm:col-span-2">
								<Field label="音频 URL（公网可访问 / oss://）">
									<TextInput
										value={audioURL}
										onChange={(e) => setAudioURL(e.target.value)}
										placeholder="https://… 或 oss://…"
									/>
								</Field>
							</div>
						)}

						<Field label={`参考音频最大时长秒（${CLONE_MIN_SEC}-${CLONE_MAX_SEC}，0=默认）`}>
							<TextInput
								type="number"
								value={maxAudio || ''}
								onChange={(e) => setMaxAudio(parseFloat(e.target.value) || 0)}
								placeholder="0"
							/>
						</Field>
						<div className="block">
							<span className="block text-xs text-paper-300/60 mb-1.5">音频预处理（降噪 / 增强）</span>
							<label className="flex items-center gap-2 text-sm text-paper-200/80">
								<input type="checkbox" checked={preprocess} onChange={(e) => setPreprocess(e.target.checked)} />
								启用
							</label>
						</div>
					</>
				)}

				{err && (
					<div className="sm:col-span-2">
						<ErrorBox>{err}</ErrorBox>
					</div>
				)}

				{result && (
					<div className="sm:col-span-2 rounded-lg border border-emerald-400/30 bg-emerald-400/5 p-3 space-y-2">
						<div className="text-sm text-emerald-300/80">
							已创建声音「{result.voice.name}」，音色 ID：{result.voice.voice}
							{result.voice.model ? `，驱动模型：${result.voice.model}` : ''}
						</div>
						{result.previewPath && (
							<audio
								controls
								autoPlay
								className="w-full h-9"
								src={`/api/voices/preview?path=${encodeURIComponent(result.previewPath)}`}
							/>
						)}
					</div>
				)}

				<div className="sm:col-span-2 flex gap-3 flex-wrap">
					<Button type="submit" variant="seal" disabled={buildMut.isPending || !canSubmit}>
						{buildMut.isPending && <Spinner />}
						{buildMut.isPending
							? stage === 'uploading'
								? '上传参考音频…'
								: stage === 'building'
									? '克隆中…'
									: '造声中…'
							: isDesign
								? '生成音色'
								: '克隆音色'}
					</Button>
					<Button type="button" variant="ghost" onClick={onClose}>
						{result ? '完成' : '取消'}
					</Button>
				</div>
			</form>
		</Modal>
	)
}
