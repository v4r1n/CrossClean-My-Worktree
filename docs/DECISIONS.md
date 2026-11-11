# DECISIONS.md - Architecture Decision Records

> บันทึกการตัดสินใจด้านสถาปัตยกรรมของ CrossClean My Worktree
> รูปแบบ ADR: Context → Decision → Consequences

---

## ADR-001: Port เป็น Go แทนการ port Swift ไป Windows

**Date:** 2026-11-11 | **Status:** Accepted

### Context
Upstream เป็น SwiftUI app (macOS only) ต้องการให้รันบน Windows ตัวเลือก:
1. Swift on Windows — core `WorktreeKit` port ได้ แต่ SwiftUI ไม่มีบน Windows (ได้แค่ CLI) และต้องลง toolchain ~1GB + VS Build Tools
2. Go — binary เดียว ไม่ต้องมี runtime, cross-compile ทุก OS จากเครื่องเดียว, toolchain มีอยู่แล้ว
3. Python + Tkinter — ต้องมี Python บนเครื่องผู้ใช้

### Decision
**เขียนใหม่เป็น Go** (stdlib only) และ **แทนที่ Swift ทั้งหมด** (ผู้ใช้เลือก) แทนการเก็บ macOS app คู่ไว้

### Consequences
- ✅ โค้ดชุดเดียวรัน Windows/macOS/Linux, release 6 zip จาก CI ตัวเดียว
- ✅ ไม่มี dependency → ไม่มี supply-chain/อัปเดต dependency ให้ดูแล
- ❌ เสีย native macOS UI (ดู ADR-002) และ diverge จาก upstream — patch จาก upstream ต้อง port มือ
- ❌ Platform-specific code (PEB, statfs, lsof) ต้องเขียนเองแยก build tag

---

## ADR-002: UI เป็น local web page (loopback HTTP server)

**Date:** 2026-11-11 | **Status:** Accepted

### Context
Go ไม่มี native GUI toolkit ใน stdlib ตัวเลือก: Wails/WebView2 (เพิ่ม dependency + CGO บางแพลตฟอร์ม), Fyne (dependency ใหญ่, CGO), หรือ server บน `127.0.0.1` แล้วเปิด browser

### Decision
**`cleanmywt` เปิด HTTP server บน 127.0.0.1 แล้วเปิด browser** UI เป็น vanilla HTML/CSS/JS embed ด้วย `go:embed`, poll `/api/state` (600ms ตอน busy, 5s ตอน idle)

### Consequences
- ✅ ไม่มี CGO, cross-compile ได้ทุก target
- ✅ UI เดียวทุก OS, dark/light ตาม system
- ❌ ต้องเปิด terminal ค้างไว้; ไม่มี native folder picker (ใช้ text field)
- ❌ Loopback port เข้าถึงได้จากทุก process/เว็บ → ต้องมี ADR-003

---

## ADR-003: Token-in-fragment + Host allow-list สำหรับ local API

**Date:** 2026-11-11 | **Status:** Accepted

### Context
API ลบโฟลเดอร์ได้ เว็บใดๆ ใน browser ยิง POST ไป `127.0.0.1:<port>` ได้ (CSRF) และ DNS rebinding ทำให้อ่าน response ได้

### Decision
- Token สุ่ม 128-bit ต่อการเปิดหนึ่งครั้ง ใส่ใน URL **fragment** (`/#token`, ไม่ถูกส่งไป server/ไม่ลง log) → JS เก็บใน `sessionStorage` แล้วส่งใน header `X-CleanMyWT-Token`
- Custom header บังคับ CORS preflight ซึ่ง server ไม่เคยตอบอนุญาต → cross-origin ส่งไม่ได้
- `Host` ต้องเป็น `127.0.0.1:<port>` หรือ `localhost:<port>` (กัน rebinding)
- ลบ/เปิดได้เฉพาะ path ที่อยู่ใน scan ล่าสุด (`State.Lookup`) — server ไม่รับ path ใดๆ จาก client ตรงๆ
- CSP `default-src 'self'`, `X-Frame-Options: DENY`

### Consequences
- ✅ เว็บอื่นและ process อื่นที่ไม่รู้ token ควบคุมไม่ได้
- ❌ Tab เก่าหลัง restart server ใช้ไม่ได้ (403) — UI แสดงข้อความให้เปิดลิงก์ใหม่, รองรับ `hashchange`

---

## ADR-004: Windows "in use" detection ผ่าน PEB

**Date:** 2026-11-11 | **Status:** Accepted

### Context
Verdict "Risky" ต้องรู้ว่ามี process ทำงานอยู่ใน worktree (macOS ใช้ `lsof`) Windows ไม่มี public API อ่าน cwd ของ process อื่น; Restart Manager ใช้กับไฟล์ไม่ใช่ directory

### Decision
อ่าน `PEB → ProcessParameters → CurrentDirectory` ด้วย `NtQueryInformationProcess` + `ReadProcessMemory` (แบบ Process Explorer) เฉพาะ 64-bit build; process ที่เปิดไม่ได้ (user อื่น/protected) ข้าม Linux ใช้ `/proc/*/cwd`, macOS/BSD ใช้ `lsof`

### Consequences
- ✅ Parity กับ macOS — กันลบ worktree ที่ agent/terminal ใช้อยู่ (บน Windows การลบโฟลเดอร์ที่มี cwd handle จะลบได้ครึ่งเดียว)
- ❌ พึ่ง undocumented struct offsets (x64: 0x20, 0x38); 32-bit build ไม่ตรวจ
- ❌ เห็นแค่ process ของ user เดียวกัน (ระบุใน README Limitations)

---

## ADR-005: Disk usage บน Windows = file size ปัดตาม cluster

**Date:** 2026-11-11 | **Status:** Accepted

### Context
macOS ใช้ `st_blocks` (allocated จริง) Windows ต้องเปิด handle ทุกไฟล์อยู่แล้วเพื่ออ่าน `NumberOfLinks` + file ID (นับ hardlink ของ pnpm ให้ถูก) แต่ allocation size จริงต้องใช้ `GetFileInformationByHandleEx` (ไม่มีใน `syscall`, ต้องเพิ่ม `golang.org/x/sys`)

### Decision
ใช้ `GetFileInformationByHandle` (มีใน stdlib) → size ปัดขึ้นตาม cluster size ของไดรฟ์ เปิดด้วย `FILE_READ_ATTRIBUTES` + `FILE_FLAG_OPEN_REPARSE_POINT` (ไม่ follow junction) และ prefix `\\?\` สำหรับ path ยาว

### Consequences
- ✅ Stdlib only, hardlink/junction ถูกต้อง
- ❌ เป็นค่าประมาณ: ไฟล์เล็กใน MFT / compressed / sparse แสดงเกินจริง (ระบุใน README)
