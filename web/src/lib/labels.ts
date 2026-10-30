// REQ-536/M343（P19）：协作模式/运行后端中文映射单源——ProjectSidePanel 选项口径收敛
//（原 ChatWindow Tag 原生枚举直接上屏；lib 单源供两侧复用）。
export const COLLAB_MODE_LABEL: Record<string, string> = {
  agent_as_tool: '主智能体调度（agent_as_tool）',
  transfer: '路由移交（transfer）',
  single: '单智能体（single）',
}

export const RUNTIME_BACKEND_LABEL: Record<string, string> = {
  inprocess: '进程内执行',
  docker: 'Docker 沙箱',
  k8s: 'K8s Pod 沙箱',
  auto: '自动检测',
}
