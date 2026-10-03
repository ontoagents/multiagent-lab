// REQ-253④（59 号 P4）：表达性签名（简化版 DL Expressivity）——规则映射非推理器计算。
// spec 受限表达力（classes/relations/instances/attributes）→ 教学信号徽标：
//   AL = 属性语言（有概念）；H = 层级继承（有 parents）；R = 对象关系；N = 原称个体（有实例）；
//   D = 数据属性（实例有 attributes）；(C) = 补记：关系带域值域声明。
// 完整 DL 表达力等级（SROIQ 等）不适用——spec_json 为轻量教学子集，签名仅做相对表达力对比信号。
import type { Spec } from '../../../api/types'

export function expressivityOf(spec: Spec | null | undefined): { sig: string; feats: string[] } {
  if (!spec) return { sig: '', feats: [] }
  const feats: string[] = []
  let sig = ''
  if ((spec.concepts?.length ?? 0) > 0) {
    sig += 'AL'
    feats.push('概念')
  }
  if ((spec.concepts ?? []).some((c) => (c.parents ?? []).length > 0)) {
    sig += 'H'
    feats.push('层级继承')
  }
  if ((spec.relations?.length ?? 0) > 0) {
    sig += 'R'
    feats.push('对象关系')
    if ((spec.relations ?? []).every((r) => r.from && r.to)) feats.push('关系带域值域')
  }
  if ((spec.instances ?? []).some((i) => (i.relations ?? []).length > 0)) feats.push('实例关系')
  if ((spec.instances?.length ?? 0) > 0) {
    sig += 'N'
    feats.push('实例')
  }
  if ((spec.instances ?? []).some((i) => Object.keys(i.attributes ?? {}).length > 0)) {
    sig += (sig ? '(' : '') + 'D' + (sig ? ')' : '')
    feats.push('数据属性')
  }
  return { sig, feats }
}
