# Project Local Path by Daemon Host - Checklist

## Decisions (Locked)
- [2026-05-13] Scope: `local path` là path trên máy host daemon/runtime, không phải máy user/browser.
- [2026-05-13] Runtime priority: dùng `local path` trước; nếu không hợp lệ thì fallback flow hiện tại `repo checkout URL`.
- [2026-05-13] Fallback policy: nếu local path lỗi do (1) không tồn tại, (2) không phải git repo, (3) permission denied => fallback `repo checkout URL`; chỉ fail cứng khi không có fallback URL hợp lệ.
- [2026-05-13] Claim payload tối thiểu: chỉ cần `preferred_workdir`.

## 1. Scope and Contract
- [x] Chốt scope: local path là path trên máy host daemon, không phải máy user/browser.
- [x] Chốt ưu tiên runtime: `local path` -> fallback `repo checkout URL`.
- [x] Chốt behavior khi path lỗi (không tồn tại, không phải git repo, không đủ quyền).
- [x] Chốt format dữ liệu trả về cho daemon task claim: chỉ `preferred_workdir`.

## 2. Data Model
- [x] Thiết kế bảng mapping `workspace_id + project_id + daemon_id -> local_path`.
- [x] Thêm cột `branch_hint` (optional) cho branch mặc định.
- [x] Thêm unique/index phù hợp để query nhanh theo `project_id + daemon_id`.
- [x] Viết migration up/down.
- [x] Cập nhật sqlc queries + generated code.

## 3. Backend API
- [x] Thêm API CRUD mapping local path (list/create/update/delete).
- [x] Validate path input: trim, non-empty, normalize path separator.
- [x] Áp policy allowlist root path (chỉ cho path trong các root cho phép).
- [x] Kiểm tra quyền: chỉ admin/owner workspace được sửa mapping.
- [x] Thêm test handler cho success/validation/permission.

## 4. Daemon Claim and Execution
- [x] Cập nhật claim payload để include `preferred_workdir` theo daemon claim.
- [x] Ở daemon runtime: nếu `preferred_workdir` hợp lệ thì dùng trực tiếp.
- [x] Nếu invalid thì fallback `multica repo checkout` URL.
- [x] Log rõ nhánh đã chọn (local path hay fallback URL) để debug.
- [x] Thêm test daemon cho cả local-path success và fallback flow.

## 5. CLI and UX
- [ ] Thêm CLI command để set/get local path mapping theo project + daemon.
- [ ] Thêm output rõ daemon nào đang map path nào.
- [ ] Thêm cảnh báo khi path chưa tồn tại trên host.
- [ ] (Optional) Thêm nút/config trong UI project settings.

## 6. Security and Safety
- [ ] Chặn path traversal/relative path nguy hiểm (`..`, symlink escape nếu cần).
- [ ] Chặn path ngoài allowlist root.
- [ ] Audit log cho hành động đổi mapping path.
- [ ] Review khả năng lộ thông tin path nội bộ trong payload/log/UI.

## 7. Compatibility and Rollout
- [ ] Backward compatible: project chưa có local path vẫn chạy URL checkout như cũ.
- [ ] Feature flag (ví dụ `LOCAL_PROJECT_PATH_ENABLED`) để rollout an toàn.
- [ ] Tài liệu migration cho môi trường đang chạy daemon cũ.

## 8. Testing Matrix
- [ ] Unit test validation path.
- [ ] Integration test handler CRUD + permission.
- [ ] Daemon integration test với path tồn tại (checkout/pull thành công).
- [ ] Daemon integration test path lỗi -> fallback URL.
- [ ] Regression test luồng GitHub/GitLab hiện tại không bị vỡ.

## 9. Docs and Runbook
- [ ] Viết docs cấu hình local path theo daemon host.
- [ ] Viết troubleshooting: path không tồn tại, quyền folder, branch mismatch.
- [ ] Viết guideline team multi-host (mỗi host có path khác nhau).

## 10. Acceptance Criteria
- [ ] Agent có thể vào đúng folder local đã map theo daemon host và chạy git pull/checkout.
- [ ] Không cần setup GitHub/GitLab integration cho use case local-first.
- [ ] Khi local path không dùng được, hệ thống fallback URL checkout ổn định.
- [ ] Có test tự động bao phủ luồng chính + lỗi thường gặp.
