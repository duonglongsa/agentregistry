import { createClient } from "@/lib/api/client"
import { listApiGovernanceAccounts, postApiGovernanceAccountsByIdRoles, getApiGovernanceMe, getApiGovernanceRecords, getApiGovernanceRecordsById, getApiGovernanceAudit, postApiGovernanceCommands, postApiGovernanceLogin, postApiGovernanceLogout } from "@/lib/api/sdk.gen"
import type { Command, GetApiGovernanceAuditData, GetApiGovernanceRecordsData } from "@/lib/api/types.gen"

// Same-origin cookies through the existing Next.js /api proxy. This client is
// separate from the public catalog client, whose base URL is cross-origin.
const client = createClient({ baseUrl: "", credentials: "same-origin" })
async function unwrap<T>(promise: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  const result = await promise
  if (result.error || !result.response.ok) {
    const error = result.error as { detail?: string }
    throw new Error(result.response.status === 401 ? "Sign in required. Your session may have expired." : error?.detail || "Unable to complete this action. Refresh and try again.")
  }
  return result.data as T
}
export const governance = {
  accounts: () => unwrap(listApiGovernanceAccounts({client})),
  assign: (id:string, roles:string[], baseVersion:number) => unwrap(postApiGovernanceAccountsByIdRoles({client,path:{id},body:{roles,baseVersion}})),
  me: () => unwrap(getApiGovernanceMe({ client })),
  login: (id: string, password: string) => unwrap(postApiGovernanceLogin({ client, body: { id, password } })),
  logout: () => unwrap(postApiGovernanceLogout({ client })),
  list: (query: GetApiGovernanceRecordsData["query"]) => unwrap(getApiGovernanceRecords({ client, query })),
  get: (id: string) => unwrap(getApiGovernanceRecordsById({ client, path: { id } })),
  command: (body: Command) => unwrap(postApiGovernanceCommands({ client, body })),
  audit: (query: GetApiGovernanceAuditData["query"]) => unwrap(getApiGovernanceAudit({ client, query })),
}
