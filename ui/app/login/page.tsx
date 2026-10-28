"use client"

import Image from "next/image"
import { useRouter } from "next/navigation"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Field } from "@/components/governance/fields"
import { governance } from "@/lib/governance-api"

export default function LoginPage() {
  const router = useRouter()
  const [checking, setChecking] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    governance.me().then(() => router.replace("/governance")).catch(() => setChecking(false))
  }, [router])

  return <main className="grid min-h-screen lg:grid-cols-[minmax(0,1fr)_minmax(420px,560px)]">
    <section className="hidden bg-muted/40 p-12 lg:flex lg:flex-col lg:justify-between">
      <Image src="/logo-light.svg" alt="Agent Registry" width={220} height={72} className="h-auto w-52 dark:hidden" priority />
      <div className="max-w-xl">
        <p className="mb-4 text-sm font-medium uppercase tracking-[0.18em] text-primary">Enterprise Agent Platform</p>
        <h1 className="text-4xl font-semibold leading-tight">Quản lý Agent, Tool và quyền phê duyệt tại một nơi.</h1>
        <p className="mt-5 text-muted-foreground">Quyền được ghi nhận tại Registry; chưa tích hợp kiểm soát thực thi.</p>
      </div>
      <p className="text-sm text-muted-foreground">Agent Registry &amp; Governance · Local demo</p>
    </section>

    <section className="flex items-center justify-center px-6 py-12 sm:px-10">
      <div className="w-full max-w-sm">
        <Image src="/logo-light.svg" alt="Agent Registry" width={190} height={64} className="mb-10 h-auto w-44 dark:hidden lg:hidden" priority />
        <h2 className="text-3xl font-semibold">Đăng nhập</h2>
        <p className="mb-8 mt-2 text-sm text-muted-foreground">Sử dụng tài khoản demo đã được cấp. Vai trò và đơn vị được xác định bởi Server.</p>
        {checking ? <p role="status">Đang kiểm tra phiên đăng nhập…</p> : <form className="space-y-5" onSubmit={async event => {
          event.preventDefault(); setBusy(true); setError("")
          const form = new FormData(event.currentTarget)
          try {
            await governance.login(String(form.get("id")), String(form.get("password")))
            router.push("/governance")
          } catch (cause) {
            setError((cause as Error).message)
          } finally {
            setBusy(false)
          }
        }}>
          <Field name="id" label="ID tài khoản" />
          <Field name="password" label="Mật khẩu" type="password" />
          {error && <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{error}</p>}
          <Button className="w-full" type="submit" disabled={busy}>{busy ? "Đang đăng nhập…" : "Đăng nhập"}</Button>
        </form>}
        <p className="mt-8 text-xs leading-relaxed text-muted-foreground">Đây là cơ chế xác thực dành cho môi trường local demo, không thay thế SSO doanh nghiệp.</p>
      </div>
    </section>
  </main>
}
