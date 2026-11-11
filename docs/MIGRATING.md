# MIGRATING - CrossClean My Worktree

> วิธีย้ายจากเวอร์ชันก่อนหน้า เรียงจากใหม่ไปเก่า

## สารบัญ

- [จาก Clean My Worktree 1.0.0 (macOS app) → 2.0.0](#จาก-clean-my-worktree-100-macos-app--200)

---

## จาก Clean My Worktree 1.0.0 (macOS app) → 2.0.0

2.0.0 เขียนใหม่ทั้งหมดเป็น Go ไม่มี `.app` แล้ว — แอปเปิดใน browser แทน ข้อมูลใน git repository ไม่ได้รับผลกระทบ (ไม่มี state ของแอปเก็บใน repo)

### 1. ติดตั้ง

| ก่อน (1.0.0) | หลัง (2.0.0) |
|--------------|--------------|
| `CleanMyWT.app` ใน `/Applications` | `cleanmywt` binary ไว้ที่ไหนก็ได้ (หรือ `go install github.com/v4r1n/CrossClean-My-Worktree/cmd/...@latest`) |
| `wtscan` ใน `/usr/local/bin` | `wtscan` ตัวใหม่ — แทนที่ตัวเดิมได้เลย ใช้ argument เดิม |
| `xattr -dr com.apple.quarantine /Applications/CleanMyWT.app` | `xattr -d com.apple.quarantine cleanmywt wtscan` |

ลบแอปเก่า (ถ้าไม่ใช้แล้ว):

```bash
rm -rf /Applications/CleanMyWT.app
defaults delete app.cleanmywt.CleanMyWT   # settings เก่า
```

### 2. Settings

Settings เก่า (`defaults` domain `app.cleanmywt.CleanMyWT`: scan target + recent folders) **ไม่ถูก import** — 2.0.0 เริ่มที่ home folder และเก็บใหม่ใน:

| OS | Path |
|----|------|
| Windows | `%AppData%\CleanMyWT\settings.json` |
| macOS | `~/Library/Application Support/CleanMyWT/settings.json` |
| Linux | `~/.config/CleanMyWT/settings.json` |

เลือก folder เดิมอีกครั้งจากเมนู **Choose folder…** — จะถูกจำใน recent folders

### 3. การใช้งานที่เปลี่ยน

| 1.0.0 | 2.0.0 |
|-------|-------|
| เปิดแอปจาก Dock | รัน `cleanmywt` — เปิด browser ให้เอง; ต้องเปิดหน้าต่าง terminal ค้างไว้ |
| ⌘O / NSOpenPanel | พิมพ์ path ใน **Choose folder…** (`~` ใช้ได้) |
| ⌘R rescan | Ctrl+R / ⌘R |
| ⌘⌫ delete marked | **Delete** หรือ Ctrl/⌘+Backspace |
| Highlight rows + Space | ไม่มี — ใช้ checkbox + shift-click |
| Badge "Repo deleted" | "Repo not found" |

### 4. wtscan

Output format เดิม ใช้ script ที่ parse output อยู่ได้ต่อ ต่างกันแค่:
- `--no-size` วางตำแหน่งไหนก็ได้
- option ที่ไม่รู้จัก → exit code 2 (เดิมถูกมองเป็น folder)
- ตัวย่อ path ใช้ `\` บน Windows (`~\code\...`)
