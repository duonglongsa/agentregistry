import type { Actor } from "@/lib/api/types.gen"

export const governanceSections = {
  agent: "Hồ sơ Agent",
  instance: "Instance",
  server: "MCP Server",
  tool: "Snapshot Tool",
  request: "Hộp thư phê duyệt",
  grant: "Quyền Registry",
  audit: "Kiểm toán",
  accounts: "Phân quyền",
} as const

export type GovernanceSection = keyof typeof governanceSections

const roleSections: Record<string, GovernanceSection[]> = {
  owner: ["agent", "instance", "request", "grant"],
  tool_owner: ["server", "tool"],
  approver: ["agent", "instance", "server", "tool", "request", "grant"],
  auditor: ["agent", "instance", "server", "tool", "request", "grant", "audit"],
  admin: ["agent", "instance", "server", "tool", "request", "grant", "audit", "accounts"],
}

export function availableGovernanceSections(actor: Actor): GovernanceSection[] {
  const allowed = new Set(actor.roles?.flatMap(role => roleSections[role] ?? []))
  return (Object.keys(governanceSections) as GovernanceSection[]).filter(section => allowed.has(section))
}

export function governanceSectionFromQuery(value: string | null, actor: Actor): GovernanceSection {
  const available = availableGovernanceSections(actor)
  return value && available.includes(value as GovernanceSection)
    ? value as GovernanceSection
    : available[0] ?? "agent"
}
