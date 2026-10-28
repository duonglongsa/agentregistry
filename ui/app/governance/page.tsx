"use client"
import { Suspense, useEffect, useState } from "react"
import { useSearchParams } from "next/navigation"
import { governance } from "@/lib/governance-api"
import type { Actor, Command, Record as GovRecord } from "@/lib/api/types.gen"
import { Button, buttonVariants } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Field } from "@/components/governance/fields"
import { CreateForm } from "@/components/governance/create-form"
import { RecordName } from "@/components/governance/record-name"
import { RecordDetail } from "@/components/governance/record-detail"
import { AccountsView } from "@/components/governance/accounts-view"
import { AuditView } from "@/components/governance/audit-view"
import { governanceLabel } from "@/lib/governance-labels"
import { governanceSectionFromQuery, governanceSections } from "@/lib/governance-navigation"

export default function GovernancePage() {
  return <Suspense fallback={<main className="container mx-auto px-6 py-8" role="status">Đang tải Governance…</main>}><GovernanceContent /></Suspense>
}

function GovernanceContent() {
  const searchParams = useSearchParams()
  const [actor, setActor] = useState<Actor | null>(null)
  const [checking, setChecking] = useState(true)
  const section = actor ? governanceSectionFromQuery(searchParams.get("section"), actor) : "agent"
  const [records, setRecords] = useState<GovRecord[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [state, setState] = useState("")
  const [search, setSearch] = useState("")
  const [selected, setSelected] = useState<GovRecord | null>(null)
  const [create, setCreate] = useState(false)
  const [choices, setChoices] = useState<GovRecord[]>([])
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const [version, setVersion] = useState(0)
  const [success, setSuccess] = useState("")
  useEffect(() => { governance.me().then(setActor).catch(() => {}).finally(() => setChecking(false)) }, [])
  useEffect(() => { setOffset(0); setState(""); setSearch(""); setSelected(null); setCreate(false); setSuccess("") }, [section])
  useEffect(() => {
    if (!actor || ["audit","accounts"].includes(section)) return
    let live = true; setLoading(true); setError("")
    governance.list({ kind: section, offset, limit: 25, state, search }).then(p => { if (live) { setRecords(p.items || []); setTotal(p.total) } }).catch(e => live && setError(e.message)).finally(() => live && setLoading(false))
    return () => { live = false }
  }, [actor, section, offset, state, search, version])
  async function select(id: string) { setLoading(true); setError(""); try { setSelected(await governance.get(id)); setCreate(false) } catch (e) { setError((e as Error).message) } finally { setLoading(false) } }
  async function newRecord() {
    setError(""); setLoading(true)
    try { const all: GovRecord[] = []; let offset = 0; while (true) { const p = await governance.list({ offset, limit: 100 }); all.push(...(p.items || [])); offset += (p.items || []).length; if (offset >= p.total || !p.items?.length) break }; setChoices(all); setCreate(true); setSelected(null) } catch (e) { setError((e as Error).message) } finally { setLoading(false) }
  }
  const canCreate = actor && ((["agent", "instance", "request"].includes(section) && actor.roles?.includes("owner")) || (["server", "tool"].includes(section) && (actor.roles?.includes("tool_owner") || actor.roles?.includes("admin"))))
  return <main className="container mx-auto max-w-7xl px-4 py-8 sm:px-6 space-y-6">
    <div className="rounded-md border border-amber-500/50 bg-amber-50 p-4 text-sm text-amber-950 dark:bg-amber-950/30 dark:text-amber-100" role="note"><strong>Chỉ phê duyệt tại Registry.</strong> Quyền được ghi nhận tại Registry; chưa tích hợp kiểm soát thực thi.</div>
    <div><h1 className="text-3xl font-semibold">{governanceSections[section]}</h1><p className="mt-2 text-muted-foreground">Quản lý hồ sơ, quyết định phê duyệt và lịch sử quyền truy cập có thể truy vết.</p></div>
    {checking ? <p role="status">Đang kiểm tra phiên…</p> : !actor ? <section className="rounded-lg border p-8 text-center"><h2 className="text-xl font-semibold">Bạn cần đăng nhập</h2><p className="my-3 text-sm text-muted-foreground">Đăng nhập để truy cập hồ sơ, phê duyệt và nhật ký kiểm toán theo phạm vi được cấp.</p><a href="/login" className={buttonVariants()}>Đi đến trang đăng nhập</a></section> : <>
      {error && <div role="alert" className="rounded border border-destructive p-4 text-destructive">{error}<Button variant="link" onClick={() => setVersion(v => v + 1)}>Thử lại</Button></div>}
      {success && <p role="status" className="text-sm">{success}</p>}
      {section === "accounts" ? <AccountsView actor={actor} /> : section === "audit" ? <AuditView /> : <>
        {section === "request" && actor.roles?.includes("approver") && !selected && !create && <ToolReviewQueue actorId={actor.id} version={version} onSelect={select} />}
        <div className="flex flex-wrap gap-3 items-end"><form onSubmit={e => { e.preventDefault(); setSearch(String(new FormData(e.currentTarget).get("search") || "")); setOffset(0) }} className="flex gap-2 items-end"><Field name="search" label="Tìm theo tên / ID" required={false} /><Button type="submit" variant="outline">Tìm kiếm</Button></form><div><label htmlFor="state-filter" className="text-sm block mb-2">Trạng thái</label><select id="state-filter" value={state} onChange={e => { setState(e.target.value); setOffset(0) }} className="h-10 rounded border bg-background px-3"><option value="">Tất cả trạng thái</option>{["draft", "registered", "pending_review", "approved", "rejected", "cancelled", "suspended", "retired", "archived", "expired", "revoked"].map(s => <option key={s} value={s}>{governanceLabel(s)}</option>)}</select></div>{canCreate && <Button onClick={newRecord} disabled={loading}>Tạo {governanceLabel(section)}</Button>}</div>
        {loading && <p role="status" aria-live="polite">Đang tải bản ghi…</p>}
        {create ? <CreateForm kind={section as Command["kind"]} records={choices} onCancel={() => setCreate(false)} onDone={() => { setCreate(false); setVersion(v => v + 1); setSuccess(section === "instance" ? "Đã đăng ký Instance." : section === "server" ? "Đã đăng ký MCP Server." : section === "tool" ? "Đã tạo Tool snapshot bản nháp. Approver có thể duyệt trong Hộp thư phê duyệt." : "Đã lưu bản nháp. Hãy gửi duyệt khi sẵn sàng.") }} /> : selected ? <><Button variant="outline" onClick={() => setSelected(null)}>Quay lại danh sách</Button><RecordDetail key={selected.id} record={selected} actor={actor} onSelect={select} onRefresh={action => { select(selected.id); setVersion(v => v + 1); setSuccess(action === "submit" ? "Đã gửi yêu cầu, đang chờ Approver duyệt." : action === "approve" ? "Đã phê duyệt tại Registry." : action === "reject" ? "Đã từ chối yêu cầu." : action === "revoke" ? "Đã thu hồi quyền tại Registry." : action === "suspend" ? "Đã tạm ngưng Agent." : action === "resume" ? "Đã khôi phục Agent." : action === "archive" ? "Đã lưu trữ bản ghi." : action === "cancel" ? "Đã hủy yêu cầu." : "Đã lưu thay đổi.") }} /></> : <>
          {!loading && !records.length ? <div className="rounded-lg border p-10 text-center"><h2 className="font-medium">Không tìm thấy {governanceSections[section]}</h2><p className="mt-2 text-sm text-muted-foreground">Hãy đổi bộ lọc hoặc tạo bản ghi mới nếu vai trò của bạn cho phép.</p></div> : <div className="overflow-x-auto rounded-lg border"><table className="w-full text-left text-sm"><thead className="bg-muted/50"><tr>{["Bản ghi", "Owner", "Phiên bản / môi trường", "Trạng thái", "Cập nhật lúc"].map(h => <th className="p-3" key={h}>{h}</th>)}</tr></thead><tbody>{records.map(r => <tr key={r.id} className="border-t"><td className="p-3"><Button variant="link" onClick={() => select(r.id)}><RecordName key={r.id} record={r} /></Button><span className="block text-xs text-muted-foreground break-all">{r.id}</span></td><td className="p-3">{r.owner}</td><td className="p-3">{r.data.snapshot || r.data.tag || r.data.environment || `bản sửa đổi ${r.version}`}</td><td className="p-3"><Badge variant="secondary">{governanceLabel(r.state)}</Badge></td><td className="p-3 whitespace-nowrap">{new Date(r.updatedAt).toLocaleString("vi-VN")}</td></tr>)}</tbody></table></div>}
          <div className="flex gap-3 items-center"><Button variant="outline" disabled={!offset || loading} onClick={() => setOffset(Math.max(0, offset - 25))}>Trước</Button><span className="text-sm">{total} bản ghi trong phạm vi của bạn</span><Button variant="outline" disabled={offset + 25 >= total || loading} onClick={() => setOffset(offset + 25)}>Tiếp</Button></div>
        </>}
      </>}
    </>}
  </main>
}

function ToolReviewQueue({ actorId, version, onSelect }: { actorId: string; version: number; onSelect: (id: string) => void }) {
  const [items, setItems] = useState<GovRecord[]>([])
  const [offset, setOffset] = useState(0)
  const [total, setTotal] = useState(0)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    let current = true
    governance.list({ kind: "tool", state: "draft", offset, limit: 25 }).then(page => {
      if (current) { setItems(page.items ?? []); setTotal(page.total) }
    }).catch(cause => { if (current) setError(cause.message) }).finally(() => { if (current) setLoading(false) })
    return () => { current = false }
  }, [offset, version])
  return <section className="rounded-lg border p-4" aria-labelledby="tool-review-heading">
    <h2 id="tool-review-heading" className="font-semibold">Tool snapshot chờ duyệt</h2>
    <p className="mt-1 text-sm text-muted-foreground">Tool Owner tạo snapshot bản nháp; Approver khác người tạo duyệt trực tiếp tại đây. Yêu cầu quyền Tool của Agent đi qua luồng gửi duyệt riêng bên dưới.</p>
    {error && <p role="alert" className="mt-2 text-sm text-destructive">{error}</p>}
    {loading ? <p role="status" className="mt-3 text-sm">Đang tải Tool…</p> : items.length ? <ul className="mt-3 divide-y">{items.map(item => <li key={item.id} className="flex items-center justify-between gap-3 py-2 text-sm"><span>{item.data.name}@{item.data.snapshot} · {item.owner}{item.owner === actorId && " · Không thể tự duyệt"}</span><Button variant="outline" size="sm" onClick={() => onSelect(item.id)}>Xem chi tiết</Button></li>)}</ul> : <p className="mt-3 text-sm text-muted-foreground">Không có Tool snapshot chờ duyệt.</p>}
    {total > 25 && <div className="mt-3 flex items-center gap-2 text-sm"><Button variant="outline" size="sm" disabled={offset === 0 || loading} onClick={() => { setLoading(true); setOffset(Math.max(0, offset - 25)) }}>Trước</Button><span>{total} Tool</span><Button variant="outline" size="sm" disabled={offset + 25 >= total || loading} onClick={() => { setLoading(true); setOffset(offset + 25) }}>Tiếp</Button></div>}
  </section>
}
