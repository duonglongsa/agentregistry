"use client"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import type { Actor } from "@/lib/api/types.gen"
import { governance } from "@/lib/governance-api"
import { governanceLabel } from "@/lib/governance-labels"
export function AccountsView({ actor }: { actor: Actor }) {
  const [accounts, setAccounts] = useState<Actor[]>([])
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const [version, setVersion] = useState(0)
  const [message, setMessage] = useState("")
  useEffect(() => { setBusy(true); governance.accounts().then(a => setAccounts(a || [])).catch(e => setError(e.message)).finally(() => setBusy(false)) }, [version])
  return <section className="space-y-4"><h2 className="text-xl font-semibold">Phân quyền · {actor.department.toUpperCase()}</h2><p className="text-sm text-muted-foreground">Tài khoản được cấp qua thiết lập cục bộ. Mọi thay đổi vai trò đều được kiểm toán và làm mất hiệu lực các phiên hiện có. Bạn không thể đổi vai trò của chính mình hoặc của đơn vị khác.</p>{busy && <p role="status">Đang tải…</p>}{error && <p role="alert" className="text-destructive">{error}</p>}{message && <p role="status">{message}</p>}
    {accounts.map(a => <form key={`${a.id}-${a.revision}`} className="space-y-3 rounded-lg border p-4" onSubmit={async e => { e.preventDefault(); const roles = new FormData(e.currentTarget).getAll("roles").map(String); setError(""); setBusy(true); try { await governance.assign(a.id, roles, a.revision); setMessage(`Đã lưu vai trò cho ${a.name}.`); setVersion(v => v + 1) } catch (e) { setError((e as Error).message) } finally { setBusy(false) } }}>
      <h3 className="font-medium">{a.name} <span className="text-sm text-muted-foreground">({a.id})</span></h3><fieldset disabled={a.id === actor.id || busy} className="flex flex-wrap gap-4"><legend className="sr-only">Vai trò của {a.name}</legend>{["owner", "tool_owner", "approver", "auditor", "admin"].map(r => <label key={r} className="flex gap-2 items-center text-sm"><input name="roles" type="checkbox" value={r} defaultChecked={a.roles?.includes(r)} />{governanceLabel(r)}</label>)}<Button type="submit" variant="outline">Lưu vai trò</Button></fieldset>
    </form>)}
  </section>
}
