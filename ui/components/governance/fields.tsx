import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { ReactNode } from "react"

export function Field({ name, label, type = "text", required = true, value, placeholder }: { name: string; label: string; type?: string; required?: boolean; value?: string; placeholder?: string }) {
  return <div className="space-y-2"><Label htmlFor={`gov-${name}`}>{label}</Label><Input id={`gov-${name}`} name={name} type={type} required={required} defaultValue={value} placeholder={placeholder} maxLength={type === "password" ? 72 : 2000} /></div>
}
export function Choice({ name, label, children, required = true }: { name: string; label: string; children: ReactNode; required?: boolean }) {
  return <div className="space-y-2"><Label htmlFor={`gov-${name}`}>{label}</Label><select id={`gov-${name}`} name={name} required={required} className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm">{children}</select></div>
}
export function LongField({ name, label, value, required = true }: { name: string; label: string; value?: string; required?: boolean }) {
  return <div className="space-y-2"><Label htmlFor={`gov-${name}`}>{label}</Label><textarea id={`gov-${name}`} name={name} required={required} defaultValue={value} rows={4} maxLength={32768} className="w-full rounded-md border border-input bg-background p-3 text-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring" /></div>
}
