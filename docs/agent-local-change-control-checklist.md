# Agent Local Change Control - Checklist

## Decisions (Locked)
- [2026-05-18] Mục 0 (Goal) đã chốt.
- [2026-05-18] Mục 1.1, 1.2 đã chốt.
- [2026-05-18] Mục 1.3 đã chốt giới hạn output:
  - `changed_files` tối đa 200 file (vượt ngưỡng => `collect_status=truncated`).
  - Tổng payload change summary tối đa 256KB (vượt ngưỡng => truncate + `collect_status=truncated`).
  - Giữ `diff_stat` đầy đủ.
  - Phase 1 không trả full patch (chỉ summary).
  - Timeout collector: 3s (timeout => `collect_status=error`, không fail task).

## 0. Goal
- [x] Khi agent chạy trực tiếp trên local project path, người dùng phải thấy được thay đổi file ở mức cơ bản để kiểm soát.
- [x] Scope MVP: hiển thị `changed_files`, `diff_stat`, và `execution_workdir` theo từng task.

## 1. Scope & Decision
- [x] Chốt phạm vi thay đổi: chỉ read-only (không edit/rollback từ UI ở phase này).
- [x] Chốt thời điểm snapshot: lấy snapshot sau khi task kết thúc (`completed`/`failed`/`blocked`).
- [x] Chốt giới hạn output: số file tối đa, kích thước diff tối đa để tránh payload quá lớn.
- [x] Chốt fallback khi không phải git repo: trả trạng thái `git_unavailable` thay vì fail task.

## 2. Backend Data Contract
- [x] Thiết kế payload kết quả task bổ sung:
  - [x] `execution_workdir`
  - [x] `git_branch`
  - [x] `head_before` / `head_after` (nếu có)
  - [x] `changed_files[]` (path + status)
  - [x] `diff_stat` (files_changed, insertions, deletions)
  - [x] `collect_status` (`ok` | `git_unavailable` | `truncated` | `error`)
- [x] Đảm bảo backward compatibility cho consumer cũ (field mới optional).

## 3. Daemon Collection Logic (BE Runtime)
- [x] Thêm collector chạy trong daemon sau execute task:
  - [x] `git rev-parse --is-inside-work-tree`
  - [x] `git status --porcelain`
  - [x] `git diff --name-status`
  - [x] `git diff --shortstat`
  - [x] `git rev-parse HEAD`
- [x] Collector chỉ đọc dữ liệu, không làm thay đổi repo.
- [x] Bổ sung guard timeout cho mỗi lệnh git.
- [x] Bổ sung truncation logic khi quá nhiều file hoặc output quá lớn.
- [x] Gắn collector output vào `TaskResult` gửi về server.

## 4. Backend API & Storage
- [x] Chốt lưu metadata ở đâu:
  - [x] Lưu trong `agent_task_queue.result` (MVP nhanh) hoặc
  - [ ] Bảng riêng `agent_task_change_summary` (nếu cần query/report nhiều).
- [x] Cập nhật response DTO cho API task detail/list để FE đọc được field mới.
- [x] Thêm test handler/service cho serialization/deserialization dữ liệu thay đổi.

## 5. Frontend (FE) - UI cơ bản
- [x] Chọn vị trí hiển thị: task detail panel / transcript dialog.
- [x] Render block "Code Changes" gồm:
  - [x] Execution folder
  - [x] Branch + HEAD
  - [x] Diff stat (+/-)
  - [x] Danh sách changed files
- [x] Trạng thái empty/error:
  - [x] Không có thay đổi
  - [x] Không phải git repo
  - [x] Truncated
- [x] Thêm copy button cho `execution_workdir` và file path (optional, low cost).

## 6. FE Optional (Phase 2 - vẫn basic)
- [ ] Expand từng file để xem patch preview (read-only unified diff).
- [ ] Giới hạn hiển thị patch theo file size / line count.

## 7. Observability & Audit
- [x] Thêm log daemon khi collect change summary:
  - [x] repo path
  - [x] collect_status
  - [x] files changed count
- [x] Thêm metric đơn giản:
  - [x] số task có change summary
  - [x] số task `git_unavailable` / `error`

## 8. Security & Safety
- [x] Redact path nếu cần trước khi hiển thị cho role không phù hợp (nếu policy yêu cầu).
- [x] Không để lộ nội dung file nhạy cảm ngoài scope MVP (phase 1 chỉ summary).
- [x] Validate path output để tránh path traversal / malformed entries từ command output.

## 9. Testing Matrix
- [x] Unit test parser cho `git status --porcelain` và `diff --shortstat`.
- [x] Daemon integration test:
  - [x] repo có thay đổi
  - [x] repo sạch
  - [x] thư mục không phải git repo
  - [x] repo rất nhiều file (truncated)
- [x] API/handler test field mới không làm vỡ endpoint cũ.
- [x] FE component test render đủ các trạng thái (`ok`, `empty`, `git_unavailable`, `error`).

## 10. Acceptance Criteria
- [x] Sau mỗi task, user xem được task đã đổi file nào.
- [x] User thấy rõ task chạy ở folder nào (`execution_workdir`).
- [x] Nếu không thu thập được git info, task vẫn hoàn thành bình thường và có reason rõ ràng.
- [x] Không có regression trên luồng task hiện tại (claim/execute/complete/fail).

## 11. Next Scope (Project-Level Git Source of Truth)
- [x] Chốt nguyên tắc: **không phụ thuộc agent output/prompt** để hiển thị file change.
- [x] Nguồn dữ liệu duy nhất cho "Code Changes" là Git command chạy phía daemon trên `execution_workdir`.
- [x] Nếu task không có `execution_workdir` hợp lệ: trả trạng thái `missing_execution_workdir` (không dùng text từ agent để suy luận).

## 12. Backend Plan (Authoritative Git Delta)
- [x] Bổ sung snapshot mốc đầu task:
  - [x] `task_git_baseline` gồm `task_id`, `execution_workdir`, `baseline_head`, `baseline_branch`, `started_at`.
  - [x] Ghi baseline ngay sau khi daemon resolve xong `execution_workdir` và trước khi execute agent.
- [x] Bổ sung collector delta theo baseline ở cuối task:
  - [x] Nếu `baseline_head` tồn tại: dùng `git diff --name-status <baseline_head>..HEAD`.
  - [x] Cộng thêm untracked từ `git status --porcelain` để không mất file mới chưa add.
  - [x] Trả `collect_status=ok|git_unavailable|truncated|error|missing_execution_workdir`.
- [x] Lưu payload kết quả chuẩn hoá vào `agent_task_queue.result`:
  - [x] `execution_workdir` luôn phải có khi task chạy local-path thành công.
  - [x] `change_summary` luôn có `collect_status` (kể cả khi lỗi).

## 13. Frontend Plan (Task + Project Level)
- [x] Task level:
  - [x] Block "Code Changes" chỉ render từ `result.change_summary` (không đọc/parse text output).
  - [x] Hiển thị rõ badge trạng thái mới `missing_execution_workdir`.
- [x] Project level:
  - [x] Thêm tab/panel "Project Changes" trong project detail.
  - [x] API trả danh sách task gần nhất có `execution_workdir` + `change_summary`.
  - [x] Group theo ngày/task để user kiểm soát thay đổi toàn project.

## 14. Verification Gates (Must Pass)
- [x] E2E local-path run:
  - [x] Task tạo file trong repo local-path. (covered by `TestCollectTaskChangeSummaryWithBaseline_UsesBaselineDelta`)
  - [x] `issue runs` có `execution_workdir=<local_path>` và `change_summary.collect_status=ok`. (covered by `TestListTasksByIssue_IncludesExecutionWorkdirAndChangeSummary`)
  - [x] UI task hiển thị file changed không dựa vào text output.
- [x] E2E non-git path:
  - [x] `collect_status=git_unavailable`, task vẫn complete/fail bình thường.
- [x] Regression:
  - [x] Không ảnh hưởng flow claim/execute/complete/fail.
  - [x] Automated checks pass: `go test ./internal/daemon ./internal/handler ./internal/service`, `pnpm -C packages/core typecheck`, `pnpm -C packages/views typecheck`, `pnpm -C packages/views test -- execution-log-section.test.tsx`.
  - [x] Baseline delta check pass: `go test ./internal/daemon -run TestCollectTaskChangeSummaryWithBaseline_UsesBaselineDelta -count=1`.

## 15. Phase 2 - Live Repo Status (Project View)
- [x] Scope: thêm "Live Repo Status" để thấy trạng thái git hiện tại của local path (không phụ thuộc task run metadata).
- [ ] Backend API:
  - [x] Thêm endpoint `GET /api/projects/{id}/live-git-status`.
  - [x] Resolve từ `project_local_repo_path` (ưu tiên mapping mới nhất).
  - [x] Trả `collect_status=ok|git_unavailable|error|missing_local_path`.
  - [x] Trả `changed_files` + `diff_stat` + `git_branch` + `head_after` + `execution_workdir`.
- [ ] Frontend:
  - [x] Gọi API mới ở Project Detail.
  - [x] Render block "Live Repo Status" tách biệt với "Project Changes (task snapshots)".
  - [x] Empty/error states rõ nghĩa (missing mapping / non-git / command error).
- [ ] Tests:
  - [x] Handler test: repo git có file changed/untracked -> trả `ok` + file list.
  - [x] Handler test: path không phải git -> `git_unavailable`.
  - [x] Typecheck + targeted tests pass.
