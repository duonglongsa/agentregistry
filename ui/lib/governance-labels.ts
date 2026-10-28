const labels: Record<string, string> = {
  agent: "Hồ sơ Agent", instance: "Instance", server: "MCP Server", tool: "Snapshot Tool",
  request: "Yêu cầu phê duyệt", grant: "Phê duyệt Registry", audit: "Nhật ký kiểm toán",
  draft: "Bản nháp", registered: "Đã đăng ký", pending_review: "Chờ duyệt", approved: "Đã duyệt",
  rejected: "Bị từ chối", cancelled: "Đã hủy", suspended: "Tạm ngưng", retired: "Ngừng sử dụng",
  archived: "Đã lưu trữ", expired: "Đã hết hạn", revoked: "Đã thu hồi",
  create: "Tạo", update: "Cập nhật", submit: "Gửi duyệt", approve: "Phê duyệt", reject: "Từ chối",
  cancel: "Hủy", revoke: "Thu hồi", suspend: "Tạm ngưng", resume: "Khôi phục", archive: "Lưu trữ",
  success: "Thành công", denied: "Bị từ chối", low: "Thấp", medium: "Trung bình", high: "Cao",
  read: "Đọc", write: "Ghi", owner: "Owner", tool_owner: "Tool Owner", approver: "Approver",
  auditor: "Auditor", admin: "Admin",
}

export function governanceLabel(value?: string) {
  return value ? labels[value] ?? value : ""
}
