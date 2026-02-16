# Linera Development Guide

Quick reference for building and developing the Linera file sharing application.

---

## 1. Initial Setup

### Fix npm Path (Windows)

If you get "npm not found" errors, run this first:

```powershell
$env:Path += ";C:\Program Files\nodejs"
```

### Install Dependencies

When cloning the project for the first time:

```bash
# Download Go modules
go mod tidy

# Install frontend packages (creates node_modules folder)
cd frontend
npm install
cd ..
```

---

## 2. Development Mode

Run the app with hot reload for active development:

```bash
wails dev
```

Changes to frontend and backend code will reload automatically.

---

## 3. Production Build

Build a single executable for distribution:

```bash
# Recommended: Optimized build without console window
wails build -ldflags "-s -w -H=windowsgui"
```

**Flags Explained:**

- `-s -w`: Strip debug symbols (reduces file size)
- `-H=windowsgui`: Hide black console window on startup

**Output Location:**

```text
build/bin/Linera.exe
```

---

## 4. Other Commands

### Clean Build

Remove old build artifacts before compiling:

```bash
wails build -clean
```

### Development Build (with console)

For debugging purposes:

```bash
wails build
```

---

## 5. Troubleshooting

### Q: Can I just use `go build`?

**A:** No. Wails projects require the frontend to be compiled and embedded into the binary.  
If `frontend/dist` is empty or missing, `go build` will fail.  
Always use `wails build` — it compiles the frontend first, then builds the Go binary.

### Q: "wails: command not found"

**A:** Install Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Q: Frontend build errors?

**A:** Ensure you've installed frontend dependencies:

```bash
cd frontend
npm install
```

---

## 6. Project Structure

```text
Antigravity/
├── app.go              # Backend logic (Go)
├── main.go             # Application entry point
├── frontend/           # Desktop GUI (Wails)
│   ├── index.html
│   └── src/
│       ├── main.js
│       └── style.css
├── assets/             # Web UI (served over HTTP)
│   ├── index.html
│   ├── script.js
│   └── style.css
└── build/bin/          # Compiled executables
    └── Linera.exe
```

---

## 7. Recent Features

- ✅ Recursive file search (search across all subdirectories)
- ✅ File preview (images, text, PDF, video, audio)
- ✅ Bulk download (multiple files as ZIP)
- ✅ Dark mode toggle
- ✅ PIN-protected uploads
- ✅ QR code for network sharing

---

**Last Updated:** February 2026  
**Framework:** Wails v2.11.0  
**Go Version:** 1.22+  
**Node Version:** 18+
