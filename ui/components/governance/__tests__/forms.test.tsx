import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { CreateForm } from "../create-form"
import { RecordDetail } from "../record-detail"
import { RecordName } from "../record-name"
import type { Actor, Record as GovRecord } from "@/lib/api/types.gen"
const command = vi.fn()
const get = vi.fn()
vi.mock("@/lib/governance-api", () => ({ governance: { command: (...args: unknown[]) => command(...args), get: (...args: unknown[]) => get(...args), list: vi.fn().mockResolvedValue({ items: [], total: 0 }) } }))
const tool: GovRecord = { id: "tool-id", kind: "tool", department: "ktud", owner: "tools", state: "draft", version: 1, data: { name: "create_issue" }, hash: "hash", createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z" }
const approver: Actor = { id: "reviewer", name: "Reviewer", department: "ktud", roles: ["approver"], revision: 1 }
beforeEach(() => vi.clearAllMocks())
describe("Governance forms", () => {
  it("distinguishes profile review from tool access and shows only reviewed tools", async () => {
    render(<CreateForm kind="request" records={[tool, { ...tool, id: "reviewed", state: "approved", data: { name: "reviewed_tool" } }]} onDone={vi.fn()} onCancel={vi.fn()} />)
    expect(screen.getByRole("option", { name: /reviewed_tool/ })).toBeInTheDocument()
    expect(screen.queryByRole("option", { name: /create_issue/ })).not.toBeInTheDocument()
    await userEvent.selectOptions(screen.getByLabelText("Loại yêu cầu"), "agent")
    expect(screen.getByLabelText("Hồ sơ Agent")).toBeInTheDocument()
    expect(screen.queryByLabelText("Snapshot Tool đã được duyệt")).not.toBeInTheDocument()
  })
  it("submits profile metadata through the generated API client wrapper", async () => {
    command.mockResolvedValue({})
    const done = vi.fn()
    render(<CreateForm kind="agent" records={[]} onDone={done} onCancel={vi.fn()} />)
    await userEvent.type(screen.getByLabelText("Tên"), "ktv-agent")
    await userEvent.type(screen.getByLabelText("Mô tả nghiệp vụ"), "Business support")
    await userEvent.type(screen.getByLabelText("Mục đích sử dụng"), "Prepare Jira issues")
    await userEvent.click(screen.getByRole("button", { name: "Lưu bản nháp" }))
    expect(command).toHaveBeenCalledWith(expect.objectContaining({ action: "create", kind: "agent", baseVersion: 0, data: expect.objectContaining({ name: "ktv-agent", purpose: "Prepare Jira issues" }) }))
    expect(done).toHaveBeenCalledOnce()
  })
  it("creates an instance from its agent without asking for the parent again", async () => {
    command.mockResolvedValue({})
    const agent: GovRecord = { ...tool, id: "agent-id", kind: "agent", data: { name: "ktv-agent" } }
    render(<CreateForm kind="instance" parent={agent.id} records={[agent]} onDone={vi.fn()} onCancel={vi.fn()} />)
    expect(screen.queryByLabelText("Agent")).not.toBeInTheDocument()
    expect(screen.getByText("ktv-agent")).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText("Tên"), "ktv-dev")
    await userEvent.click(screen.getByRole("button", { name: "Lưu bản nháp" }))
    expect(command).toHaveBeenCalledWith(expect.objectContaining({ action: "create", kind: "instance", parent: "agent-id", data: expect.objectContaining({ environment: "dev", location: "platform" }) }))
  })
  it("hides review actions from a self approver and from an auditor", () => {
    const { rerender } = render(<RecordDetail record={tool} actor={{ ...approver, id: "tools" }} onRefresh={vi.fn()} onSelect={vi.fn()} />)
    expect(screen.queryByRole("button", { name: "Phê duyệt" })).not.toBeInTheDocument()
    rerender(<RecordDetail record={tool} actor={{ ...approver, roles: ["auditor"] }} onRefresh={vi.fn()} onSelect={vi.fn()} />)
    expect(screen.queryByRole("button", { name: "Phê duyệt" })).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Lưu trữ" })).not.toBeInTheDocument()
  })
  it("offers review to a distinct approver", () => {
    render(<RecordDetail record={tool} actor={approver} onRefresh={vi.fn()} onSelect={vi.fn()} />)
    expect(screen.getByRole("button", { name: "Phê duyệt" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Từ chối" })).toBeInTheDocument()
  })
  it("shows the Agent and Instance in a grant name", async () => {
    get.mockImplementation((id: string) => Promise.resolve(id === "instance-id"
      ? { ...tool, id, kind: "instance", parent: "agent-id", data: { name: "ktv-dev", environment: "dev" } }
      : { ...tool, id, kind: "agent", data: { name: "ktv-agent" } }))
    render(<RecordName record={{ ...tool, id: "grant-id", kind: "grant", parent: "instance-id", data: { name: "create_issue", snapshot: "v1" } }} />)
    expect(await screen.findByText("ktv-agent / dev → create_issue@v1")).toBeInTheDocument()
  })
})
