# CLAUDE.md - CrossClean My Worktree Project Guide

> เอกสารสำหรับ AI Coding Assistant เพื่อทำความเข้าใจโปรเจค CrossClean My Worktree
> อ่านคู่กับ [docs/MEMORY.md](docs/MEMORY.md) (state ล่าสุด) และ [docs/DECISIONS.md](docs/DECISIONS.md) (ADR) ก่อนแก้โค้ด

## 📋 ภาพรวมโปรเจค

**CrossClean My Worktree** หาและลบ git worktree ที่ถูกลืม (เช่นที่ AI agent สร้างไว้ใน `.claude/worktrees/`) แล้วคืนพื้นที่ดิสก์
เป็น cross-platform port ของ [poomsc/Clean-My-Worktree](https://github.com/poomsc/Clean-My-Worktree) (SwiftUI, macOS only)
เขียนใหม่ทั้งหมดเป็น Go ให้รันบน **Windows (หลัก)**, macOS และ Linux

| รายการ | ข้อมูล |
|--------|--------|
| **Repo** | [v4r1n/CrossClean-My-Worktree](https://github.com/v4r1n/CrossClean-My-Worktree) |
| **Upstream** | [poomsc/Clean-My-Worktree](https://github.com/poomsc/Clean-My-Worktree) (Swift, v1.0.0) |
| **Version** | 2.0.0 (ดู [docs/CHANGELOG.md](docs/CHANGELOG.md)) |
| **Module** | `github.com/v4r1n/CrossClean-My-Worktree` |
| **Language** | Go 1.22 language version (toolchain ใหม่กว่าได้) — **stdlib only, ไม่มี dependency** |
| **Binaries** | `cleanmywt` (local web UI), `wtscan` (CLI) |
| **Runtime dep** | `git` บน `PATH` |
| **License** | MIT (copyright upstream คงไว้ใน `LICENSE`) |

---

## 🏗️ โครงสร้างโปรเจค

```
cmd/
  cleanmywt/main.go     # start loopback server, เปิด browser, auto-scan target ที่ save ไว้
  wtscan/main.go        # CLI report; parse flag เอง (flag อยู่ตำแหน่งไหนก็ได้)
internal/
  wtkit/                # core ไม่มี UI — CLI กับ app ใช้ร่วมกัน
    models.go           # Repository, Worktree, Status, Usage, Verdict (+ Verdict ladder)
    paths.go            # Real/Key/IsWithin/Abbreviate — Key = lower-case บน Windows
    shell.go            # Run/Git, GIT_OPTIONAL_LOCKS=0, RepoArgs
    scanner.go          # ScanRepos (WalkDir), skippedNames/skippedPaths ต่อ OS, resolveGitFile
    inspector.go        # LoadRepository, InspectStatus, parseWorktreeList/parseStatus
    usage.go            # Measure: hardlink นับครั้งเดียว, ไม่ข้าม filesystem
    remover.go          # Remove (guard .git file), DeleteBranch, removeAll (read-only บน Windows)
    pipeline.go         # FullScan: walk → load → status(8 workers) → measure(4 workers) + Events
    presentation.go     # Badges, Detail, BranchLabel, FormatBytes (decimal เหมือน Finder)
    *_windows.go        # PEB cwd, CreateFile stat + \\?\ long path, GetDiskFreeSpaceEx, hide console
    *_unix.go / *_linux.go  # lstat blocks, lsof (mac) / /proc (linux), statfs
  app/
    server.go           # HTTP API + guard (Host allow-list + token header + CSP)
    state.go            # State (mutex), scan generation, delete job, sharedVolume
    snapshot.go         # Snapshot/Row ที่ส่งให้ UI (verdict/badges คำนวณฝั่ง Go)
    settings.go         # %AppData%/CleanMyWT/settings.json (target + recent 5)
    system*.go          # OpenURL/Reveal/OpenTerminal ต่อ OS (ไม่ผ่าน shell)
    web/                # index.html + app.css + app.js (vanilla, embed ด้วย go:embed)
  testrepo/             # test helper: fixture repo + Isolate() (HOME/APPDATA → temp) — ใช้จาก _test.go เท่านั้น
scripts/dist.sh         # cross-compile 6 targets → dist/*.zip + SHA256SUMS.txt
scripts/coverage.sh     # go test + coverage gate 80% (MIN_COVERAGE), ไม่นับ internal/testrepo
e2e/                    # Playwright browser E2E: build binary จริง → spawn ต่อ test ด้วย home แยก
.github/workflows/build.yml  # jobs: test (gofmt/vet/race/coverage) + e2e × win/mac/linux → release (smoke linux zip); tag v* → Release
docs/                   # MEMORY, CHANGELOG, MIGRATING, DECISIONS
```

---

## 🛠️ คำสั่งที่ใช้บ่อย

```bash
go build -o build/ ./cmd/...        # build/cleanmywt(.exe) + build/wtscan(.exe)
go test ./...                       # ต้องมี git; unit + integration + HTTP E2E + CLI E2E
./scripts/coverage.sh               # coverage gate ≥80% (CI ใส่ -race)
cd e2e && npm ci && npx playwright install chromium && npx playwright test   # browser E2E
go vet ./... && GOOS=linux go vet ./... && GOOS=darwin go vet ./...   # เช็ค build tag ทุก OS จากเครื่องเดียว
gofmt -l .                          # ต้องว่าง
./build/cleanmywt --no-browser --port 47811   # smoke test UI
./scripts/dist.sh v2.0.0            # release zips (ต้องมี zip — Windows ไม่มี, ให้ CI ทำ)
```

Smoke test UI: ตั้ง `APPDATA` ไปที่ temp ก่อนรัน ไม่งั้น settings ของ user จริงถูกเขียนทับ

---

## 📐 Conventions

- **Core อยู่ใน `internal/wtkit` เท่านั้น** — app กับ CLI ต้องได้ผลเหมือนกัน (verdict/badge คำนวณใน Go ไม่ใช่ JS)
- **Platform code แยกไฟล์ด้วย build tag/suffix** (`_windows.go`, `_unix.go`, `_linux.go`, `_other.go`) — อย่าใช้ `runtime.GOOS` กับ syscall ที่ไม่มีทุก OS
- **เทียบ path ด้วย `Key()` / `IsWithin()` เสมอ** — Windows case-insensitive, git คืน `C:/...` (forward slash)
- **ห้าม follow symlink/junction** ทั้งตอน scan, measure, ลบ (pnpm บน Windows ใช้ junction)
- **ห้ามส่ง path เข้า shell** (`cmd /c`, `sh -c`) — ใช้ `exec.Command` + `cmd.Dir` (เคยมี injection ผ่าน `&` ในชื่อโฟลเดอร์)
- **Server**: ทุก `/api/*` ต้องมี `X-CleanMyWT-Token`; ลบ/เปิดได้เฉพาะ path ที่อยู่ใน scan ล่าสุด (`State.Lookup`)
- **JS**: ทุก string จาก server ผ่าน `esc()`; ไม่มี inline script/style (CSP `default-src 'self'`); ใช้ `[hidden]` ไม่ใช่ `style="display"`
- **Stdlib only** — ถ้าจะเพิ่ม dependency ต้องเขียน ADR
- **Tests**: AAA, ใช้ git repo จริงใน `t.TempDir()`; อะไรที่ลบไฟล์ต้องมี test กับ repo ทิ้ง
- **Test ที่แตะ home/settings ต้องเรียก `testrepo.Isolate(t)`** — ไม่งั้น scan/เขียน settings ของ user จริง
- **OS-specific command (browser/explorer/terminal)** ทดสอบผ่าน seam `goos`/`lookPath`/`startCommand` ใน `internal/app/system.go` — ห้ามเปิดหน้าต่างจริงใน test
- **cmd/*: logic อยู่ใน `run(ctx, args, stdout, stderr, …) int`**, `main()` แค่ wire signal + `os.Exit`
- **Coverage ≥80% total** — CI fail ถ้าต่ำกว่า; E2E Playwright เป็น dependency เดียว (อยู่ใน `e2e/` ไม่แตะ Go module)
- **Commits**: [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/), commit ตรงลง `main`
- **Versioning**: [SemVer 2.0.0](https://semver.org/spec/v2.0.0.html), changelog แบบ [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/)

---

## ⚠️ Gotchas

- `go test` ผ่านเรื่อง path ยาว >260 แม้ไม่มี fix ถ้าเครื่องเปิด `LongPathsEnabled` — fix `longPath()` จำเป็นสำหรับเครื่องที่ไม่เปิด
- Windows size = file size ปัดขึ้นตาม cluster (ประมาณการ) — ไฟล์เล็กมากอยู่ใน MFT จริงๆ ใช้ 0
- In-use บน Windows อ่าน PEB (offset x64: `ProcessParameters` 0x20, `CurrentDirectory` 0x38) — 32-bit build คืน nil
- Go < 1.22.6 build macOS binary ที่ dyld ปฏิเสธ (`missing LC_UUID`) → CI ใช้ `go-version: stable` ไม่ใช่ go.mod
- `flag` package หยุดที่ arg แรกที่ไม่ใช่ flag → `wtscan` parse เอง
- Module เป็น go 1.22 → ห้ามใช้ `t.Context()`/`t.Chdir()` (1.24+) ใน test
- Heredoc ใน Git Bash ยุบ backslash คู่เหลือตัวเดียว → เขียนไฟล์ที่มี Windows path literal ด้วย Write tool
- `explorer.exe` exit 1 แม้สำเร็จ → `Start()` ไม่ `Run()`
- Navigate เปลี่ยนแค่ hash ไม่ reload หน้า → `app.js` ฟัง `hashchange` เพื่อรับ token ใหม่

---

## Session End Protocol
เมื่อจบ session หรือ feature สำเร็จ ให้อัปเดตไฟล์เหล่านี้เสมอ:
- [ ] docs/MEMORY.md — state ปัจจุบัน, สิ่งที่เปลี่ยน
- [ ] docs/CHANGELOG.md — version + สิ่งที่ทำ (Keep a Changelog + SemVer)
- [ ] docs/DECISIONS.md — ADR ใหม่ (ถ้ามี architectural decision)
- [ ] docs/MIGRATING.md — breaking change (ถ้ามี)
- [ ] README.md — ภาพรวม ฟีเจอร์ วิธีติดตั้ง การใช้งาน ให้ตรงกับสถานะล่าสุด
- [ ] CLAUDE.md — stack/convention ที่เปลี่ยน
- [ ] `gofmt -l .` ว่าง → `go vet` (windows/linux/darwin) → `./scripts/coverage.sh` (≥80%) → `e2e` Playwright → git commit (Conventional Commits) + push; tag เมื่อ release เท่านั้น
