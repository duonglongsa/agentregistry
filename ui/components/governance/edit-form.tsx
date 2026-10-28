"use client"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { governance } from "@/lib/governance-api"
import type { Command, Record as GovRecord } from "@/lib/api/types.gen"
import { Choice, Field, LongField } from "./fields"
import { governanceLabel } from "@/lib/governance-labels"
export function EditForm({ record, onDone, onCancel }: { record: GovRecord; onDone: () => void; onCancel: () => void }) {
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  return <form className="space-y-4 rounded border p-4" onSubmit={async e => { e.preventDefault(); setError(""); setBusy(true); const f = new FormData(e.currentTarget); const text = (s: string) => String(f.get(s) || "").trim(); try {
    const data = { ...record.data, purpose: text("purpose"), description: text("description") }
    if (record.kind !== "request") data.risk = text("risk") as typeof data.risk
    if (record.kind === "tool") { data.inputSchema = JSON.parse(text("schema")); data.classification = text("classification") }
    if (record.kind === "request" && data.requestType === "access") { data.operations = text("operations").split(",").map(s => s.trim()); data.constraints = JSON.parse(text("constraints")); data.expiresAt = new Date(text("expiry")).toISOString() }
    await governance.command({ action: "update", kind: record.kind as Command["kind"], id: record.id, baseVersion: record.version, data }); onDone()
  } catch (e) { setError(e instanceof SyntaxError ? "Hãy nhập JSON hợp lệ." : (e as Error).message) } finally { setBusy(false) } }}>
    <h3 className="font-medium">Sửa bản nháp · bản sửa đổi {record.version}</h3>
    {record.kind !== "request" && <><LongField name="description" label="Mô tả nghiệp vụ" value={record.data.description} /><Choice name="risk" label={`Mức rủi ro (hiện tại: ${governanceLabel(record.data.risk)})`}><option value={record.data.risk}>{governanceLabel(record.data.risk)}</option>{["low", "medium", "high"].filter(r => r !== record.data.risk).map(r => <option key={r} value={r}>{governanceLabel(r)}</option>)}</Choice></>}
    {record.kind !== "tool" && <LongField name="purpose" label="Mục đích" value={record.data.purpose} />}
    {record.kind === "tool" && <><Choice name="classification" label="Phân loại"><option value={record.data.classification}>{governanceLabel(record.data.classification)}</option><option value={record.data.classification === "read" ? "write" : "read"}>{governanceLabel(record.data.classification === "read" ? "write" : "read")}</option></Choice><LongField name="schema" label="JSON Schema đầu vào" value={JSON.stringify(record.data.inputSchema, null, 2)} /></>}
    {record.kind === "request" && record.data.requestType === "access" && <><Field name="operations" label="Thao tác (phân cách bằng dấu phẩy)" value={record.data.operations?.join(", ")} /><LongField name="constraints" label="JSON điều kiện phạm vi" value={JSON.stringify(record.data.constraints, null, 2)} /><Field name="expiry" label="Hết hạn (ISO timestamp kèm múi giờ)" value={record.data.expiresAt} /></>}
    {error && <p role="alert" className="text-destructive">{error}</p>}<div className="flex gap-2"><Button disabled={busy}>Lưu thay đổi</Button><Button type="button" variant="outline" onClick={onCancel}>Hủy chỉnh sửa</Button></div>
  </form>
}
