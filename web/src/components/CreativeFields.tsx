import type { CreativeCatalog, CreativeStyle } from '../types'
import { Field, Select, TextArea } from './ui'

/**
 * CreativeFields 创作控制参数表单（系列级）。
 * 完全由 GET /api/creative-catalog 的注册表驱动：参数、选项、默认名一律读自 catalog，
 * 前端不写死任何参数名/选项。
 *
 * 信息架构（§26）：只保留「一眼能选的画面观感项」（画风 / 运镜强度 / 平台时长档位）。
 * 讲述口味（结构、钩子、节奏、落点）是项目内建的唯一默认，不再做成可调参数。
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

// 单个枚举/文本参数的渲染。
function KnobControl({
  catalog,
  values,
  onChange,
  disabled,
  k,
}: {
  catalog: CreativeCatalog
  values: CreativeStyle
  onChange: (next: CreativeStyle) => void
  disabled: boolean
  k: CreativeCatalog['knobs'][number]
}) {
  const current = knobMap(values, catalog)
  if (k.type === 'text') {
    return (
      <Field label={k.label}>
        <TextArea
          value={current[k.key]}
          disabled={disabled}
          maxLength={k.max_length}
          placeholder={k.default_label}
          onChange={(e) => onChange(setKnobValue(values, k.key, e.target.value))}
        />
        {k.help && <span className="block mt-1 text-xs text-paper-300/40 leading-relaxed">{k.help}</span>}
      </Field>
    )
  }
  return (
    <Field label={k.label}>
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
      {k.help && <span className="block mt-1 text-xs text-paper-300/40 leading-relaxed">{k.help}</span>}
    </Field>
  )
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

  return (
    <div className="grid sm:grid-cols-2 gap-4">
      {catalog.knobs.map((k) => (
        <KnobControl key={k.key} k={k} catalog={catalog} values={values} onChange={onChange} disabled={disabled} />
      ))}
    </div>
  )
}