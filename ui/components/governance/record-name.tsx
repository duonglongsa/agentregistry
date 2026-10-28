"use client"
import { useEffect, useState } from "react"
import { governance } from "@/lib/governance-api"
import type { Record as GovRecord } from "@/lib/api/types.gen"
// Older local demo drafts may predate the denormalized immutable display refs.
// Resolve their pinned tool ID for display, never infer approval from catalog state.
export function RecordName({ record }: { record: GovRecord }) {
  const [tool, setTool] = useState<GovRecord | null>(null)
  const [context, setContext] = useState("")
  useEffect(() => { if (!record.tool || record.data.name) return; let live = true; governance.get(record.tool).then(t => live && setTool(t)).catch(() => {}); return () => { live = false } }, [record.tool, record.data.name])
  useEffect(() => {
    if (!record.parent || !["grant", "request"].includes(record.kind) || record.data.requestType === "agent") return
    let live = true
    governance.get(record.parent).then(async instance => {
      const agent = instance.parent ? await governance.get(instance.parent) : null
      if (live) setContext(`${agent?.data.name || "Agent"} / ${instance.data.environment || "Instance"}`)
    }).catch(() => {})
    return () => { live = false }
  }, [record.id, record.kind, record.parent, record.data.requestType])
  const name = record.data.name || (tool ? `${tool.data.name}@${tool.data.snapshot}` : `${record.data.requestType || record.kind} · ${record.id.slice(0, 8)}`)
  return <>{context ? `${context} → ${name}${record.data.snapshot ? `@${record.data.snapshot}` : ""}` : name}</>
}
