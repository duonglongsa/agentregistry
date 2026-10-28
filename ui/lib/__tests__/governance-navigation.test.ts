import { describe, expect, it } from "vitest"
import type { Actor } from "@/lib/api/types.gen"
import { availableGovernanceSections, governanceSectionFromQuery } from "../governance-navigation"

const actor = (roles: string[]): Actor => ({ id: "demo", name: "Demo", department: "ktud", roles, revision: 1 })

describe("governance navigation", () => {
  it.each([
    { roles: ["owner"], visible: ["agent", "instance", "request", "grant"] },
    { roles: ["tool_owner"], visible: ["server", "tool"] },
    { roles: ["approver"], visible: ["agent", "instance", "server", "tool", "request", "grant"] },
    { roles: ["auditor"], visible: ["agent", "instance", "server", "tool", "request", "grant", "audit"] },
    { roles: ["admin"], visible: ["agent", "instance", "server", "tool", "request", "grant", "audit", "accounts"] },
  ])("shows the right sections for $roles", ({ roles, visible }) => {
    expect(availableGovernanceSections(actor(roles))).toEqual(visible)
  })

  it("combines roles and falls back from a hidden or unknown section", () => {
    const user = actor(["owner", "auditor"])
    expect(availableGovernanceSections(user)).toContain("audit")
    expect(governanceSectionFromQuery("accounts", user)).toBe("agent")
    expect(governanceSectionFromQuery("invalid", actor(["tool_owner"]))).toBe("server")
  })
})
