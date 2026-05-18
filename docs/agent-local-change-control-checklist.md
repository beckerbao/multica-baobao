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
