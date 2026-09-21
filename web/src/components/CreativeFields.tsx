import type { CreativeCatalog, CreativeStyle } from '../types'
import { Field, Select, TextArea } from './ui'

/**
 * CreativeFields 创作控制参数表单（系列级）。
 * 完全由 GET /api/creative-catalog 的注册表驱动：参数、选项、默认名一律读自 catalog，
 * 前端不写死任何参数名/选项；渲染画风与指令也只是「其中的一项」，不做特判。
 */

// 按键读值：未设置视为空串（＝跟随内置默认）。用 Record 访问以避免在逻辑里写死参数名。
export function knobValue(values: CreativeStyle, key: string): string {
  return (values as Record<string, string | undefined>)[key] ?? ''
}

// 按键写值：展开保留 values 中其它已有 key（不因重新渲染丢字段）。
export function setKnobValue(values: CreativeStyle, key: string, value: string): CreativeStyle {
  return { ...values, [key]: value } as CreativeStyle
}

// 把 values 归一成 {knobKey: value}（key 全集取自 catalog.knobs，缺省＝空串）。
export function knobMap(values: CreativeStyle, catalog: CreativeCatalog): Record<string, string> {
  const out: Record<string, string> = {}
  for (const k of catalog.knobs) out[k.key] = knobValue(values, k.key)
  return out
}

export default function CreativeFields({
  catalog,
  values,
  onChange,
  disabled = false,
}: {
  catalog: CreativeCatalog | undefined
  values: CreativeStyle
  onChange: (next: CreativeStyle) => void
  disabled?: boolean
}) {
  if (!catalog) {
    return <div className="text-sm text-paper-300/40 py-2">创作设置加载中…</div>
  }

  const keys = catalog.knobs.map((k) => k.key)
  const current = knobMap(values, catalog)
  const presetKey = values.preset ?? ''
  const preset = catalog.presets.find((p) => p.key === presetKey)
  // 与所选预设逐项比较（预设未列出的参数按空串），不一致即说明用户在预设基础上改过。
  const tweaked = !!preset && keys.some((k) => current[k] !== (preset.values[k] ?? ''))

  const applyPreset = (key: string) => {
    if (key === '') {
      // 跟随默认：把「上一个预设」列出的参数清空回内置默认，其余保留（含用户手写的指令）。
      const cleared: Record<string, string> = {}
      for (const pk of Object.keys(preset?.values ?? {})) cleared[pk] = ''
      onChange({ ...values, ...cleared, preset: undefined })
      return
    }
    const p = catalog.presets.find((x) => x.key === key)
    if (!p) return
    // 套用预设：合并预设列出的值，此后逐项仍可改。
    onChange({ ...values, ...p.values, preset: p.key })
  }

  return (
    <div className="space-y-4">
      <Field label="创作预设">
        <div className="flex items-center gap-3">
          <Select value={presetKey} disabled={disabled} onChange={(e) => applyPreset(e.target.value)}>
            <option value="">跟随默认（不套用）</option>
            {catalog.presets.map((p) => (
              <option key={p.key} value={p.key}>
                {p.name}
              </option>
            ))}
          </Select>
          {tweaked && <span className="shrink-0 text-xs text-gold-500/80">已在预设基础上微调</span>}
        </div>
      </Field>

      <div className="grid sm:grid-cols-2 gap-4">
        {catalog.knobs.map((k) => (
          <Field key={k.key} label={k.label}>
            {k.type === 'text' ? (
              <TextArea
                value={current[k.key]}
                disabled={disabled}
                maxLength={k.max_length}
                placeholder={k.default_label}
                onChange={(e) => onChange(setKnobValue(values, k.key, e.target.value))}
              />
            ) : (
              <Select
                value={current[k.key]}
                disabled={disabled}
                onChange={(e) => onChange(setKnobValue(values, k.key, e.target.value))}
              >
                <option value="">跟随默认：{k.default_label}</option>
                {k.options.map((o) => (
                  <option key={o.key} value={o.key}>
                    {o.label}
                  </option>
                ))}
              </Select>
            )}
            {k.help && <span className="block mt-1 text-xs text-paper-300/40 leading-relaxed">{k.help}</span>}
          </Field>
        ))}
      </div>
    </div>
  )
}