"use client"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Field, Choice, LongField } from "./fields"
import { governance } from "@/lib/governance-api"
import type { Command, Record as GovRecord, Payload } from "@/lib/api/types.gen"
import { governanceLabel } from "@/lib/governance-labels"

const risks = <>{["low", "medium", "high"].map(r => <option key={r} value={r}>{governanceLabel(r)}</option>)}</>
export function CreateForm({ kind, records, onDone, onCancel, parent, initialRequestType }: { kind: Command["kind"]; records: GovRecord[]; onDone: () => void; onCancel: () => void; parent?: string; initialRequestType?: "agent" | "access" }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [requestType, setRequestType] = useState(initialRequestType ?? "access")
  const options = (type: string, state?: string) => records.filter(r => r.kind === type && (!state || r.state === state)).map(r => <option key={r.id} value={r.id}>{r.parent && `${records.find(p=>p.id===r.parent)?.data.name || r.parent.slice(0,8)} / `}{r.data.name || r.id}{(r.data.snapshot || r.data.tag) && `@${r.data.snapshot || r.data.tag}`} {r.data.environment && `(${r.data.environment})`} · {governanceLabel(r.state)}</option>)
  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(""); setBusy(true)
    const form = new FormData(event.currentTarget)
    const text = (key: string) => String(form.get(key) || "").trim()
    try {
      const data: Payload = { name: text("name"), description: text("description"), purpose: text("purpose"), risk: (text("risk") || undefined) as Payload["risk"] }
      if (kind === "instance") Object.assign(data, { environment: text("environment"), location: text("location"), endpoint: text("endpoint"), identity: { issuer: text("issuer"), subject: text("subject"), clientReference: text("clientReference") } })
      if (kind === "server") Object.assign(data, { tag: text("tag"), endpoint: text("endpoint") })
      if (kind === "agent") {data.tag = text("tag");data.registrationScope=text("registrationScope") as typeof data.registrationScope}
      if (kind === "tool") Object.assign(data, { snapshot: text("snapshot"), classification: text("classification"), inputSchema: JSON.parse(text("schema")) })
      if (kind === "request") {
        data.requestType = requestType; data.name = ""
        if (requestType === "access") Object.assign(data, { operations: text("operations").split(",").map(s => s.trim()), constraints: JSON.parse(text("constraints")), expiresAt: new Date(text("expiry")).toISOString() })
      }
      await governance.command({ action: "create", kind, baseVersion: 0, parent: text("parent") || parent, tool: text("tool"), data })
      onDone()
    } catch (e) { setError(e instanceof SyntaxError ? "Hãy nhập JSON hợp lệ cho schema và điều kiện phạm vi." : (e as Error).message) } finally { setBusy(false) }
  }
  return <section className="rounded-lg border bg-card p-6" aria-label={`Tạo ${governanceLabel(kind)}`}><h2 className="mb-4 text-xl font-semibold">Tạo {governanceLabel(kind)}</h2>
    <form onSubmit={submit} className="space-y-4">
      {kind === "request" && <div className="space-y-2"><label htmlFor="request-type">Loại yêu cầu</label><select id="request-type" value={requestType} onChange={e => setRequestType(e.target.value as "agent" | "access")} disabled={!!parent} className="ml-3 rounded border bg-background p-2"><option value="access">Phê duyệt quyền truy cập Tool</option><option value="agent">Duyệt hồ sơ Agent</option></select></div>}
      {kind === "agent" && <Choice name="registrationScope" label="Phạm vi đăng ký"><option value="internal">Agent do MB kiểm soát</option><option value="outside_mb">Bên ngoài / SaaS — ghi nhận ngoài phạm vi Governance</option></Choice>}
      {kind !== "request" && <Field name="name" label="Tên" placeholder={kind === "tool" ? "create_issue" : "ktv-agent"} />}
      {(kind === "agent" || kind === "server") && <Field name="tag" label="Phiên bản Catalog / tag" value="latest" />}
      {(kind === "agent" || kind === "tool" || kind === "server") && <LongField name="description" label="Mô tả nghiệp vụ" />}
      {(kind === "agent" || kind === "request") && <LongField name="purpose" label="Mục đích sử dụng" />}
      {(kind === "agent" || kind === "tool") && <Choice name="risk" label="Mức rủi ro">{risks}</Choice>}
      {kind === "server" && <Field name="endpoint" label="Endpoint (chỉ là metadata)" type="url" required={false} />}
      {kind === "instance" && <>
        {parent ? <p className="text-sm">Agent: <strong>{records.find(r => r.id === parent)?.data.name || parent}</strong></p> : <Choice name="parent" label="Agent">{options("agent")}</Choice>}
        <Choice name="environment" label="Môi trường"><option>dev</option><option>test</option><option>prod</option></Choice>
        <Choice name="location" label="Vị trí thực thi"><option value="platform">Platform</option><option value="unit_infrastructure">Hạ tầng đơn vị</option><option value="workstation">Máy trạm</option></Choice>
        <Field name="endpoint" label="Endpoint (không bắt buộc)" required={false} type="url" />
        <Field name="issuer" label="Identity issuer (metadata, không bắt buộc)" required={false} /><Field name="subject" label="Identity subject (không bắt buộc)" required={false} /><Field name="clientReference" label="Tham chiếu Client (không bắt buộc; không chứa thông tin xác thực)" required={false} />
      </>}
      {kind === "tool" && <>
        <Choice name="parent" label="MCP Server / phiên bản Catalog">{options("server", "registered")}</Choice>
        <Field name="snapshot" label="Phiên bản snapshot bất biến" placeholder="v1" />
        <Choice name="classification" label="Phân loại thao tác"><option value="read">Đọc</option><option value="write">Ghi</option></Choice>
        <LongField name="schema" label="JSON Schema đầu vào (chỉ xác thực metadata)" value={'{"type":"object","properties":{"project":{"type":"string"}},"required":["project"],"additionalProperties":false}'} />
      </>}
      {kind === "request" && <>
        {parent ? <p className="text-sm">{requestType === "agent" ? "Hồ sơ Agent" : "Instance của Agent"}: <strong>{records.find(r => r.id === parent)?.data.name || records.find(r => r.id === parent)?.data.environment || parent}</strong></p> : <Choice name="parent" label={requestType === "agent" ? "Hồ sơ Agent" : "Instance của Agent"}>{options(requestType === "agent" ? "agent" : "instance")}</Choice>}
        {requestType === "access" && <>
          <Choice name="tool" label="Snapshot Tool đã được duyệt">{options("tool", "approved")}</Choice>
          <Field name="operations" label="Thao tác yêu cầu (phân cách bằng dấu phẩy)" placeholder="create_issue" />
          <LongField name="constraints" label="Điều kiện phạm vi (JSON object chứa chuỗi)" value={'{"project":"DEMO"}'} />
          <Field name="expiry" label="Hết hạn (giờ địa phương)" type="datetime-local" />
        </>}
      </>}
      {error && <p role="alert" className="text-destructive">{error}</p>}
      <div className="flex gap-2"><Button disabled={busy} type="submit">{busy ? "Đang lưu…" : "Lưu bản nháp"}</Button><Button type="button" variant="outline" onClick={onCancel}>Hủy</Button></div>
    </form></section>
}
