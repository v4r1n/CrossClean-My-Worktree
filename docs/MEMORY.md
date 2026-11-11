# MEMORY.md - CrossClean My Worktree AI Context

> State ล่าสุดของโปรเจคสำหรับ AI session ถัดไป — อัปเดตทุกครั้งตาม Session End Protocol ใน [CLAUDE.md](../CLAUDE.md)

---

## 📌 สถานะปัจจุบัน

| รายการ | สถานะ |
|--------|-------|
| **Version** | 2.0.0 (ยังไม่ tag / ยังไม่ release) |
| **Branch** | `main` → push ไป `v4r1n/CrossClean-My-Worktree` |
| **Upstream** | `poomsc/Clean-My-Worktree` v1.0.0 (history 3 commit ของ upstream อยู่ใน repo นี้) |
| **Tests** | `go test ./...` ผ่าน local (Windows 11, go1.26.2) + CI ubuntu/macos/windows (`-race`) |
| **Vet** | ผ่าน windows / linux / darwin |
| **Cross-compile** | ผ่าน 6 targets (windows/darwin/linux × amd64/arm64) |
| **Smoke test** | Windows เท่านั้น — scan/verdict/mark/delete+branch/reveal ผ่านใน browser |

---

## ✅ สิ่งที่ implement เสร็จแล้ว

- Port `WorktreeKit` (Swift) → `internal/wtkit` ครบ: scan, worktree list, status, verdict ladder, badges, measure (hardlink), remove (guard `.git` file + fallback), delete branch `-d`
- Platform layer: Windows (PEB cwd, CreateFile stat, long path, free space, hide console), Linux (`/proc`), macOS (`lsof`, statfs)
- `cleanmywt` web UI ครบ feature หลักของแอปเดิม (sidebar repo, filter, search, sort, mark/shift-click, mark safe, delete dialog + ack risky + branch, report, row menu)
- Security: token + Host allow-list + CSP + scan-scoped paths (ADR-003)
- `wtscan` CLI output format เดิม
- CI matrix + `scripts/dist.sh` release zips
- Code review (code-reviewer agent): 0 CRITICAL/HIGH; แก้แล้วทุกข้อ — terminal injection, long path, measured-freed ผิดไดรฟ์, disk query ใต้ lock, badge "Repo not found"

---

## 🔄 TODO / Pending

### ยังไม่ได้ verify
- รันจริงบน macOS / Linux แบบใช้งาน (UI) — มีแค่ unit tests ผ่าน CI
- CI run แรก (`300e4b2`): Windows ผ่าน; Linux+macOS fail จาก test in-use เปราะ (child git ไม่อยู่ใน repo → exit ทันที) + macOS `dyld: missing LC_UUID` (Go 1.22.0) → แก้แล้ว (CI ใช้ Go stable, test ทำ `git init` + รอ child ตอบ); run ที่ 2 เขียวครบ ubuntu/macos/windows + release job (`dist.sh` สร้าง zip ได้)
- `longPath()` fix — เครื่อง dev เปิด `LongPathsEnabled` จึง test ผ่านทั้งก่อน/หลัง fix
- ปุ่ม "Open in terminal" ยังไม่ได้กดทดสอบจริง
- ยังไม่เคยดาวน์โหลด zip จาก CI ไปรันบนเครื่องอื่น

### Medium
- Native folder picker (ตอนนี้พิมพ์ path) — อาจใช้ PowerShell `FolderBrowserDialog` / `osascript` / `zenity`
- `--version` flag ใน binaries (ตอนนี้ version อยู่แค่ชื่อ zip)
- Home scan บน Windows ช้า (~86k folders/40s ใน profile นี้) — พิจารณา skip `AppData\Local` บางส่วนเพิ่ม

### Low
- Keyboard: highlight rows + Space (มีใน Swift เดิม)
- `docs/promo.png` ยังเป็นภาพ macOS app เดิม

---

## 🏗️ Architecture Patterns ที่ใช้

### 1. Core / UI แยกชั้น
`internal/wtkit` ไม่รู้จัก HTTP; `FullScan(ctx, Options, Events)` ส่ง callback `Progress/Listed/Updated` (serialised ด้วย mutex ภายใน) — CLI ใช้ `Events{}` ว่าง, app ใช้ callback อัปเดต `State`

### 2. Scan generation
`State.scanGen` เพิ่มทุกครั้งที่ start/stop; callback ของ scan เก่าถูกทิ้งใน `apply()` (เช็ค gen + `ctx.Err()`) — lock order: pipeline mu → State.mu

### 3. View model ฝั่ง server
`Snapshot`/`Row` คำนวณ verdict, badges, detail, branchLabel ใน Go → JS แค่ render (กัน logic drift ระหว่าง CLI กับ UI)

### 4. Platform split
ไฟล์ `_windows.go` / `_unix.go` / `_linux.go` / `_other.go` ใช้ฟังก์ชันชื่อเดียวกัน (`ProcessWorkingDirectories`, `AvailableBytes`, `newStatter`, `hideWindow`, `newConsole`)

---

## ⚠️ Known Issues / Gotchas

- Windows sizes เป็นค่าประมาณ (ADR-005)
- Orphan detection: repo บนไดรฟ์ที่ถอดออกถูกมองเป็น orphan → verdict Review, ลบได้หลัง ack (พฤติกรรมเดียวกับ Swift เดิม; เปลี่ยนแค่ข้อความ)
- `history.replaceState` ลบ query string ด้วย (ไม่มีผล — แอปไม่ใช้ query)

---

## 📋 Session Notes (ล่าสุด)

### 2026-11-11 — Initial cross-platform port
- Clone upstream → ถามผู้ใช้: เลือก **Go + local web UI** และ **แทนที่ Swift ทั้งหมด**
- เขียน wtkit + app + CLI + tests (TDD บางส่วน: test เขียนพร้อม impl, ไม่ได้ RED ก่อนทุกข้อ)
- Smoke test ใน built-in browser กับ fixture repo ใน scratchpad: พบและแก้ 2 bug UI (`[hidden]` ถูก `display:flex` ทับ, token เก่าหลัง restart)
- พบ `flag` package หยุด parse → `wtscan` parse เอง
- Code review → แก้ 5 ข้อ
- เปลี่ยน module path เป็น `github.com/v4r1n/CrossClean-My-Worktree`, เพิ่มเครดิต upstream ใน README
- เพิ่ม CLAUDE.md + docs/ (MEMORY, CHANGELOG, MIGRATING, DECISIONS); commit + push `main` ไม่ tag
