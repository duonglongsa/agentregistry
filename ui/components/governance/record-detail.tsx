"use client"
import { useEffect, useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { governance } from "@/lib/governance-api"
import type { Actor, Command, Record as GovRecord } from "@/lib/api/types.gen"
import { RecordName } from "./record-name"
import { EditForm } from "./edit-form"
import { LongField } from "./fields"
import { CreateForm } from "./create-form"
import { governanceLabel } from "@/lib/governance-labels"

const fieldLabels: Record<string, string> = {
  name: "Tên", description: "Mô tả nghiệp vụ", purpose: "Mục đích sử dụng", risk: "Mức rủi ro",
  tag: "Phiên bản Catalog", namespace: "Đơn vị Catalog", environment: "Môi trường", location: "Vị trí thực thi",
  endpoint: "Endpoint", snapshot: "Phiên bản Tool", classification: "Phân loại thao tác",
  operations: "Thao tác được yêu cầu", constraints: "Điều kiện phạm vi", expiresAt: "Hết hạn",
  requestType: "Loại yêu cầu", serverReference: "MCP Server / phiên bản", registrationScope: "Phạm vi đăng ký",
  identity: "Tham chiếu định danh", inputSchema: "JSON Schema đầu vào", targetHash: "Hash đối tượng được duyệt",
}

function displayValue(key: string, value: unknown): string {
  if (key === "requestType") return value === "agent" ? "Duyệt hồ sơ Agent" : "Quyền truy cập Tool"
  if (key === "registrationScope") return value === "internal" ? "Agent nội bộ MB" : "Bên ngoài phạm vi Governance"
  if (key === "location") return ({ platform: "Platform", unit_infrastructure: "Hạ tầng đơn vị", workstation: "Máy trạm" } as Record<string, string>)[String(value)] ?? String(value)
  if (["expiresAt", "Thời điểm quyết định", "Thời điểm thu hồi"].includes(key)) return new Date(String(value)).toLocaleString("vi-VN")
  if (Array.isArray(value)) return value.join(", ")
  if (typeof value === "object") return JSON.stringify(value, null, 2)
  return governanceLabel(String(value))
}

export function RecordDetail({ record, actor, onRefresh, onSelect }: { record: GovRecord; actor: Actor; onRefresh: (action?: Command["action"]) => void; onSelect: (id: string) => void }) {
  const [editing,setEditing]=useState(false)
  const [tab, setTab] = useState("profile")
  const [children, setChildren] = useState<GovRecord[]>([])
  const [offset, setOffset] = useState(0)
  const [total, setTotal] = useState(0)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const [action, setAction] = useState<Command["action"] | null>(null)
  const [creating, setCreating] = useState<"instance" | "request" | null>(null)
  const [choices, setChoices] = useState<GovRecord[]>([])
  const [childrenVersion, setChildrenVersion] = useState(0)
  const [message, setMessage] = useState("")
  useEffect(() => {
    if (tab === "profile") return
    let live = true
    governance.list({ parent: record.id, kind: tab, offset, limit: 25 }).then(p => { if (live) { setChildren(p.items || []); setTotal(p.total) } }).catch(e => live && setError(e.message)).finally(() => live && setLoading(false))
    return () => { live = false }
  }, [record.id, record.version, tab, offset, childrenVersion])
  async function beginCreate(kind: "instance" | "request") {
    setError("")
    setLoading(true)
    try {
      const all: GovRecord[] = []
      let cursor = 0
      while (true) {
        const page = await governance.list({ offset: cursor, limit: 100 })
        all.push(...(page.items ?? []))
        cursor += page.items?.length ?? 0
        if (cursor >= page.total || !page.items?.length) break
      }
      setChoices(all)
      setCreating(kind)
    } catch (cause) { setError((cause as Error).message) } finally { setLoading(false) }
  }
  const has = (role: string) => actor.roles?.includes(role)
  const own = record.owner === actor.id
  const actions: Command["action"][] = []
  if (record.kind === "request" && own && has("owner")) {
    if (record.state === "draft") actions.push("submit")
    if (["draft", "pending_review"].includes(record.state)) actions.push("cancel")
  }
  if (has("approver") && !own && ((record.kind === "request" && record.state === "pending_review") || (record.kind === "tool" && record.state === "draft"))) actions.push("approve", "reject")
  if (has("approver") || has("admin")) {
    if (record.kind === "grant" && record.state === "approved") actions.push("revoke")
    if (record.kind === "agent" && record.state === "approved") actions.push("suspend")
    if (record.kind === "agent" && record.state === "suspended") actions.push("resume")
    if (["agent", "server", "tool"].includes(record.kind) && !["retired", "archived"].includes(record.state)) actions.push("archive")
  }
  const details = Object.entries({ Owner: record.owner, "Đơn vị": record.department.toUpperCase(), "Trạng thái": record.state, "Bản sửa đổi": record.version, "Người duyệt": record.decisionBy, "Thời điểm quyết định": record.decidedAt, "Lý do": record.reason, "Người thu hồi":record.revokedBy,"Thời điểm thu hồi":record.revokedAt, ...record.data }).filter(([key, value]) => value !== undefined && value !== "" && value !== null && !["targetHash", "inputSchema"].includes(key))
  return <section className="rounded-lg border bg-card p-6 space-y-5">
    <div className="flex flex-wrap items-center gap-3"><h2 className="text-xl font-semibold"><RecordName key={record.id} record={record} /></h2><Badge variant="secondary">{governanceLabel(record.state)}</Badge></div>
    <p className="break-all text-xs text-muted-foreground">{governanceLabel(record.kind)} · {record.id}</p>
    {record.kind === "grant" && <p className="text-sm">Chỉ phê duyệt tại Registry. Bản ghi này không xác nhận quyền truy cập Runtime.</p>}
    <div className="flex flex-wrap gap-2">{own && record.state==="draft" && ["agent","tool","request"].includes(record.kind) && <Button variant="outline" onClick={()=>setEditing(true)}>Sửa bản nháp</Button>}{actions.map(a => <Button key={a} variant={["revoke", "suspend", "archive", "reject"].includes(a) ? "destructive" : "outline"} onClick={() => setAction(a)}>{governanceLabel(a)}</Button>)}</div>
    {message && <p role="status" className="text-sm">{message}</p>}
    {editing && <EditForm record={record} onCancel={()=>setEditing(false)} onDone={()=>{setEditing(false);onRefresh()}} />}
    <Tabs value={tab} onValueChange={v => { setLoading(true);setError("");setTab(v); setOffset(0) }}>
      <TabsList className="flex flex-wrap h-auto"><TabsTrigger value="profile">Hồ sơ & snapshot</TabsTrigger>
        {record.kind === "agent" && <><TabsTrigger value="instance">Instance</TabsTrigger><TabsTrigger value="request">Duyệt hồ sơ</TabsTrigger></>}
        {record.kind === "instance" && <><TabsTrigger value="request">Yêu cầu truy cập</TabsTrigger><TabsTrigger value="grant">Phê duyệt Registry</TabsTrigger></>}
        {record.kind === "server" && <TabsTrigger value="tool">Snapshot Tool</TabsTrigger>}
      </TabsList>
      <TabsContent value="profile">{record.kind === "grant" && <p className="rounded-md border bg-muted/40 p-3 text-sm"><strong>Quyền được chấp thuận tại Registry</strong> cho Instance <span className="font-mono">{record.parent?.slice(0, 8)}</span>, dùng Tool <strong>{record.data.name}{record.data.snapshot && `@${record.data.snapshot}`}</strong> trong môi trường {record.data.environment}. Người duyệt: {record.decisionBy || "—"}.</p>}<dl className="divide-y">{details.map(([key, value]) => <div key={key} className="grid gap-2 py-3 sm:grid-cols-[180px_1fr]"><dt className="text-sm text-muted-foreground">{fieldLabels[key] || key}</dt><dd className="text-sm whitespace-pre-wrap break-all">{displayValue(key, value)}</dd></div>)}</dl>
        <div className="flex flex-wrap gap-2">{[["Bản ghi cha", record.parent], ["Snapshot Tool", record.tool], ["Yêu cầu nguồn", record.source]].filter(([, id]) => id).map(([name, id]) => <Button variant="link" key={name} onClick={() => onSelect(id!)}>{name}: {id?.slice(0, 8)}</Button>)}</div>
        <details className="mt-4 text-xs text-muted-foreground"><summary className="cursor-pointer">Chi tiết kỹ thuật</summary><dl className="mt-2 space-y-2"><div><dt>ID bản ghi</dt><dd className="break-all font-mono">{record.id}</dd></div><div><dt>Hash snapshot</dt><dd className="break-all font-mono">{record.hash}</dd></div>{record.data.targetHash && <div><dt>Hash đối tượng được duyệt</dt><dd className="break-all font-mono">{record.data.targetHash}</dd></div>}{record.data.inputSchema !== undefined && <div><dt>JSON Schema đầu vào</dt><dd className="whitespace-pre-wrap font-mono">{JSON.stringify(record.data.inputSchema, null, 2)}</dd></div>}</dl></details>
      </TabsContent>
      {tab !== "profile" && <TabsContent value={tab}>
        {own && has("owner") && ((record.kind === "agent" && tab === "instance") || (record.kind === "agent" && tab === "request" && record.state === "draft") || (record.kind === "instance" && tab === "request")) && <Button variant="outline" onClick={() => beginCreate(tab as "instance" | "request")} disabled={loading}>{tab === "instance" ? "Tạo Instance cho Agent" : record.kind === "agent" ? "Tạo yêu cầu duyệt hồ sơ" : "Tạo yêu cầu quyền Tool"}</Button>}
        {creating && <CreateForm kind={creating} parent={record.id} initialRequestType={record.kind === "agent" ? "agent" : "access"} records={choices} onCancel={() => setCreating(null)} onDone={() => { setCreating(null); setChildrenVersion(value => value + 1); setMessage(creating === "instance" ? "Đã đăng ký Instance." : "Đã tạo yêu cầu bản nháp. Hãy gửi duyệt khi sẵn sàng.") }} />}
        {loading ? <p role="status">Đang tải…</p> : error ? <p role="alert">{error}</p> : children.length ? <ul className="divide-y">{children.map(r => <li key={r.id} className="py-3"><Button variant="link" onClick={() => onSelect(r.id)}>{r.data.name || r.data.requestType || r.id} · {r.data.environment} · {governanceLabel(r.state)}</Button></li>)}</ul> : <p className="py-6 text-muted-foreground">Không có bản ghi {governanceLabel(tab)} cho {governanceLabel(record.kind)} này.</p>}
        <div className="flex items-center gap-3"><Button variant="outline" disabled={!offset || loading} onClick={() => setOffset(Math.max(0, offset - 25))}>Trước</Button><span className="text-sm">{total} bản ghi</span><Button variant="outline" disabled={offset + 25 >= total || loading} onClick={() => setOffset(offset + 25)}>Tiếp</Button></div>
      </TabsContent>}
    </Tabs>
    {action && <ActionDialog action={action} record={record} onClose={() => setAction(null)} onDone={() => { setAction(null); onRefresh(action) }} />}
  </section>
}
function ActionDialog({ action, record, onDone, onClose }: { action: Command["action"]; record: GovRecord; onDone: () => void; onClose: () => void }) {
  const ref = useRef<HTMLDialogElement>(null)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  useEffect(() => { const dialog = ref.current; dialog?.showModal(); return () => dialog?.close() }, [])
  return <dialog ref={ref} onCancel={onClose} className="w-full max-w-lg rounded-lg border bg-background p-6 text-foreground backdrop:bg-black/50" aria-labelledby="action-title">
    <h2 id="action-title" className="text-xl font-semibold mb-3">Xác nhận: {governanceLabel(action)}</h2><p className="mb-4 text-sm">Thao tác này chỉ thay đổi bản ghi Registry. Chưa kết nối cơ chế thực thi trên Runtime.</p>
    <form onSubmit={async e => { e.preventDefault(); const reason = String(new FormData(e.currentTarget).get("reason") || "").trim(); setBusy(true); setError(""); try { await governance.command({ action, kind: record.kind as Command["kind"], id: record.id, baseVersion: record.version, reason, data: { name: "" } }); onDone() } catch (e) { setError((e as Error).message) } finally { setBusy(false) } }} className="space-y-4">
      <LongField name="reason" label="Lý do" required={["reject", "revoke", "suspend", "archive"].includes(action)} />
      {error && <p role="alert" className="text-destructive">{error}</p>}
      <div className="flex gap-2"><Button disabled={busy} type="submit">{busy ? "Đang lưu…" : `Xác nhận ${governanceLabel(action).toLowerCase()}`}</Button><Button variant="outline" type="button" onClick={onClose} disabled={busy}>Hủy</Button></div>
    </form></dialog>
}
