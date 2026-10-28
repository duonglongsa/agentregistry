"use client"

import { useEffect, useState } from "react"
import Link from "next/link"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { governance } from "@/lib/governance-api"
import { governanceLabel } from "@/lib/governance-labels"
import type { Actor, Record as GovRecord } from "@/lib/api/types.gen"
import { RecordDetail } from "./record-detail"

const categories = [
  { kind: "agent", label: "Agent" },
  { kind: "server", label: "MCP Server" },
  { kind: "tool", label: "Tool snapshot" },
] as const

export function GovernanceCatalog() {
  const [actor, setActor] = useState<Actor | null>(null)
  const [kind, setKind] = useState<(typeof categories)[number]["kind"]>("agent")
  const [items, setItems] = useState<GovRecord[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [search, setSearch] = useState("")
  const [selected, setSelected] = useState<GovRecord | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [version, setVersion] = useState(0)

  useEffect(() => { governance.me().then(setActor).catch(error => setError(error.message)) }, [])
  useEffect(() => {
    let current = true
    governance.list({ kind, search, offset, limit: 25 }).then(page => {
      if (current) { setItems(page.items ?? []); setTotal(page.total) }
    }).catch(error => { if (current) setError(error.message) }).finally(() => { if (current) setLoading(false) })
    return () => { current = false }
  }, [kind, search, offset, version])

  async function select(id: string) {
    setError("")
    try { setSelected(await governance.get(id)) } catch (error) { setError((error as Error).message) }
  }

  return <main className="container mx-auto max-w-7xl space-y-6 px-4 py-8 sm:px-6">
    <div>
      <h1 className="text-3xl font-semibold">Danh mục Registry</h1>
      <p className="mt-2 text-sm text-muted-foreground">Agent, MCP Server và Tool snapshot trong phạm vi đơn vị của bạn. Các phiên bản đã được tham chiếu được giữ ổn định trong Catalog.</p>
    </div>
    <div className="flex flex-wrap gap-2 border-b pb-4" aria-label="Loại tài nguyên">
      {categories.map(category => <Button key={category.kind} variant={kind === category.kind ? "default" : "ghost"} aria-current={kind === category.kind ? "page" : undefined} onClick={() => { setLoading(true); setError(""); setKind(category.kind); setOffset(0); setSelected(null); setSearch("") }}>{category.label}</Button>)}
    </div>
    {selected ? <>
      <Button variant="outline" onClick={() => setSelected(null)}>Quay lại Danh mục</Button>
      {actor && <RecordDetail key={selected.id} record={selected} actor={actor} onSelect={select} onRefresh={() => { select(selected.id); setVersion(value => value + 1) }} />}
    </> : <>
      <form className="flex max-w-lg gap-2" onSubmit={event => { event.preventDefault(); setLoading(true); setError(""); setOffset(0); setSearch(String(new FormData(event.currentTarget).get("search") ?? "").trim()) }}>
        <label className="sr-only" htmlFor="catalog-search">Tìm theo tên hoặc ID</label>
        <input id="catalog-search" name="search" type="search" placeholder="Tìm theo tên hoặc ID" className="h-10 min-w-0 flex-1 rounded-md border border-input bg-background px-3 text-sm" />
        <Button type="submit">Tìm kiếm</Button>
      </form>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {loading ? <p role="status">Đang tải Danh mục…</p> : items.length ? <div className="divide-y rounded-lg border">
        {items.map(item => <button key={item.id} type="button" onClick={() => select(item.id)} className="flex w-full flex-wrap items-center justify-between gap-3 px-4 py-4 text-left hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          <span className="min-w-0"><span className="block font-medium">{item.data.name || item.id}</span><span className="block text-xs text-muted-foreground">{item.data.tag || item.data.snapshot || ""} {item.data.description && `· ${item.data.description}`}</span></span>
          <Badge variant="secondary">{governanceLabel(item.state)}</Badge>
        </button>)}
      </div> : <div className="rounded-lg border p-10 text-center text-sm text-muted-foreground">Chưa có tài nguyên phù hợp trong đơn vị này. <Link href={`/governance?section=${kind}`} className="text-primary underline">Mở mục quản trị</Link></div>}
      <div className="flex items-center gap-3 text-sm"><Button variant="outline" disabled={offset === 0 || loading} onClick={() => { setLoading(true); setOffset(Math.max(0, offset - 25)) }}>Trước</Button><span>{total} tài nguyên</span><Button variant="outline" disabled={offset + 25 >= total || loading} onClick={() => { setLoading(true); setOffset(offset + 25) }}>Tiếp</Button></div>
      <p className="text-xs text-muted-foreground">Skill và Prompt upstream hiện chưa được mở trong chế độ Governance vì API catalog cũ chưa có lọc theo đơn vị.</p>
    </>}
  </main>
}
