"use client"
import { useEffect, useState } from "react"
import { governance } from "@/lib/governance-api"
import { Button } from "@/components/ui/button"
import { Field, Choice } from "./fields"
import type { Event as GovEvent } from "@/lib/api/types.gen"
import { governanceLabel } from "@/lib/governance-labels"

export function AuditView() {
  const [filters, setFilters] = useState<{ actor?: string; action?: string; parent?: string; state?: string; from?: string; until?: string }>({})
  const [offset, setOffset] = useState(0)
  const [items, setItems] = useState<GovEvent[]>([])
  const [total, setTotal] = useState(0)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)
  useEffect(() => { let live = true; governance.audit({ ...filters, offset, limit: 25 }).then(p => { if (live) { setItems(p.items || []); setTotal(p.total) } }).catch(e => live && setError(e.message)).finally(() => live && setLoading(false)); return () => { live = false } }, [filters, offset])
  return <section className="space-y-5"><h2 className="text-xl font-semibold">Kiểm toán Governance</h2><p className="text-sm text-muted-foreground">Lịch sử Registry bền vững trong phạm vi đơn vị của bạn. Database Administrator vẫn là ranh giới tin cậy.</p>
    <form className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3" onSubmit={e => { e.preventDefault(); setLoading(true);setError(""); const f = new FormData(e.currentTarget); const value = (s: string) => String(f.get(s) || ""); setFilters({ actor: value("actor"), parent: value("parent"), action: value("action"), state: value("state"), from: value("from") ? new Date(value("from")).toISOString() : "", until: value("until") ? new Date(value("until")).toISOString() : "" }); setOffset(0) }}>
      <Field name="actor" label="ID người thao tác" required={false} /><Field name="parent" label="ID Agent / instance / đối tượng" required={false} />
      <Choice name="action" label="Thao tác" required={false}><option value="">Tất cả thao tác</option>{["create", "update", "submit", "approve", "reject", "cancel", "revoke", "suspend", "resume", "archive"].map(a => <option key={a} value={a}>{governanceLabel(a)}</option>)}</Choice>
      <Choice name="state" label="Kết quả" required={false}><option value="">Tất cả kết quả</option><option value="success">Thành công</option><option value="denied">Bị từ chối</option></Choice>
      <Field name="from" label="Từ (giờ địa phương)" type="datetime-local" required={false} /><Field name="until" label="Đến (giờ địa phương)" type="datetime-local" required={false} />
      <div className="flex gap-2"><Button type="submit">Áp dụng bộ lọc</Button><a className="inline-flex items-center rounded-md border px-4 text-sm" href={`/api/governance/audit.csv?${new URLSearchParams(filters)}`}>Xuất CSV</a></div>
    </form>
    {loading ? <p role="status">Đang tải nhật ký kiểm toán…</p> : error ? <p role="alert" className="text-destructive">{error}</p> : !items.length ? <p className="py-8 text-muted-foreground">Không có sự kiện phù hợp với bộ lọc.</p> : <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b">{["Thời gian (UTC)", "Người thao tác", "Thao tác", "Đối tượng / bản sửa đổi", "Kết quả", "Lý do"].map(h => <th className="p-3" key={h}>{h}</th>)}</tr></thead><tbody>{items.map(v => <tr key={v.id} className="border-b"><td className="p-3 whitespace-nowrap">{v.at}</td><td className="p-3">{v.actor}</td><td className="p-3">{governanceLabel(v.action)}</td><td className="p-3 break-all">{v.object} / {v.revision}<span className="block text-xs text-muted-foreground">Correlation ID: {v.correlation}</span></td><td className="p-3">{governanceLabel(v.result)}</td><td className="p-3">{v.reason}</td></tr>)}</tbody></table></div>}
    <div className="flex gap-3 items-center"><Button variant="outline" disabled={!offset || loading} onClick={() => {setLoading(true);setOffset(Math.max(0, offset - 25))}}>Trước</Button><span className="text-sm">{total} sự kiện</span><Button variant="outline" disabled={offset + 25 >= total || loading} onClick={() => {setLoading(true);setOffset(offset + 25)}}>Tiếp</Button></div>
  </section>
}
