"use client"

import Link from "next/link"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { useTheme } from "next-themes"
import { useEffect, useState, useSyncExternalStore } from "react"
import { ChevronDown, Moon, Sun } from "lucide-react"
import { toast } from "sonner"
import Image from "next/image"
import { Button, buttonVariants } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { governance } from "@/lib/governance-api"
import { governanceLabel } from "@/lib/governance-labels"
import { availableGovernanceSections, governanceSections, type GovernanceSection } from "@/lib/governance-navigation"
import type { Actor } from "@/lib/api/types.gen"

const groups: { label: string; sections: GovernanceSection[] }[] = [
  { label: "Agent", sections: ["agent", "instance"] },
  { label: "MCP & Tool", sections: ["server", "tool"] },
  { label: "Phê duyệt", sections: ["request"] },
  { label: "Quyền Registry", sections: ["grant"] },
  { label: "Kiểm toán", sections: ["audit"] },
  { label: "Phân quyền", sections: ["accounts"] },
]

export function Navigation() {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const router = useRouter()
  const { theme, setTheme } = useTheme()
  const mounted = useSyncExternalStore(() => () => {}, () => true, () => false)
  const [actor, setActor] = useState<Actor | null>(null)
  const [checkingSession, setCheckingSession] = useState(true)
  const [signingOut, setSigningOut] = useState(false)

  useEffect(() => {
    let current = true
    governance.me().then(user => { if (current) setActor(user) }).catch(() => { if (current) setActor(null) }).finally(() => { if (current) setCheckingSession(false) })
    return () => { current = false }
  }, [pathname])

  async function signOut() {
    setSigningOut(true)
    try {
      await governance.logout()
      setActor(null)
      router.push("/login")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Không thể đăng xuất. Hãy thử lại.")
    } finally {
      setSigningOut(false)
    }
  }

  if (pathname === "/login") return null

  const allowed = actor ? availableGovernanceSections(actor) : []
  const activeSection = searchParams.get("section") ?? "agent"

  return <header className="sticky top-0 z-50 border-b bg-background">
    <nav className="container mx-auto flex h-14 items-center gap-4 px-4 sm:px-6" aria-label="Điều hướng chính">
      <Link href="/" className="shrink-0 rounded-md px-1 py-1" aria-label="Về Danh mục">
        <Image src={mounted && theme === "dark" ? "/logo-dark.svg" : "/logo-light.svg"} alt="Agent Registry" width={180} height={60} className="h-10 w-auto" />
      </Link>

      <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto whitespace-nowrap">
        <HeaderLink href="/" active={pathname === "/"}>Danh mục</HeaderLink>
        {actor && groups.map(group => {
          const sections = group.sections.filter(section => allowed.includes(section))
          if (!sections.length) return null
          const active = pathname === "/governance" && sections.includes(activeSection as GovernanceSection)
          if (sections.length === 1) return <HeaderLink key={group.label} href={`/governance?section=${sections[0]}`} active={active}>{group.label}</HeaderLink>
          return <DropdownMenu key={group.label}>
            <DropdownMenuTrigger asChild>
              <button type="button" className={`inline-flex h-10 items-center gap-1 rounded-md px-3 text-sm font-medium hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${active ? "text-foreground" : "text-muted-foreground"}`} aria-current={active ? "page" : undefined}>
                {group.label}<ChevronDown className="h-3.5 w-3.5" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              {sections.map(section => <DropdownMenuItem key={section} asChild><Link href={`/governance?section=${section}`}>{governanceSections[section]}</Link></DropdownMenuItem>)}
            </DropdownMenuContent>
          </DropdownMenu>
        })}
        {actor?.roles?.includes("admin") && <HeaderLink href="/deployed" active={pathname === "/deployed"}>Triển khai</HeaderLink>}
      </div>

      <div className="ml-auto flex shrink-0 items-center gap-1">
        {!checkingSession && (actor ? <>
          <div className="max-w-24 text-right text-xs leading-tight sm:max-w-48 xl:max-w-60">
            <p className="truncate font-semibold" title={actor.name}>{actor.name}</p>
            <p className="truncate text-muted-foreground" title={`${actor.department.toUpperCase()} · ${actor.roles?.map(governanceLabel).join(", ")}`}>
              {actor.department.toUpperCase()}<span className="hidden sm:inline"> · {actor.roles?.map(governanceLabel).join(", ")}</span>
            </p>
          </div>
          <Button variant="ghost" size="sm" onClick={signOut} disabled={signingOut}>Đăng xuất</Button>
        </> : <Link href="/login" className={buttonVariants({ variant: "ghost", size: "sm" })}>Đăng nhập</Link>)}
        {mounted && <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => setTheme(theme === "dark" ? "light" : "dark")} title={`Chuyển sang giao diện ${theme === "dark" ? "sáng" : "tối"}`} aria-label={`Chuyển sang giao diện ${theme === "dark" ? "sáng" : "tối"}`}>
          {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </Button>}
      </div>
    </nav>
  </header>
}

function HeaderLink({ href, active, children }: { href: string; active: boolean; children: React.ReactNode }) {
  return <Link href={href} aria-current={active ? "page" : undefined} className={`inline-flex h-10 items-center rounded-md px-3 text-sm font-medium transition-colors hover:bg-accent ${active ? "text-foreground" : "text-muted-foreground hover:text-foreground"}`}>{children}</Link>
}
