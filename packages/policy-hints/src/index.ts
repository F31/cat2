// 策略拒绝 Reason 码 → 用户可读提示。后端只下发机器可读码，文案集中在这里维护。

const hints: Record<string, string> = {
  'sensitive.word.hit': '内容包含敏感词，请修改后再提交',
  'moderation.rejected': '内容未通过审核，请查看具体原因',
  'publish.restricted': '账号当前处于限制期，暂时无法发布',
  'quota.exceeded': '操作过于频繁，请稍后再试',
  'account.inactive': '账号状态异常，请联系客服',
  'realname.required': '该操作需要先完成实名认证',
}

/** 按原因码取提示文案；未登记的码返回兜底文案。 */
export function hintFor(reasonCode: string | undefined | null): string {
  if (reasonCode && hints[reasonCode]) return hints[reasonCode]
  return '操作被拒绝，请稍后再试'
}

export function registerHint(code: string, text: string): void {
  hints[code] = text
}
