package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"embed"
	"encoding/binary"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/skip2/go-qrcode"
)

//go:embed assets/*
var assets embed.FS

const AppVersion = "v6.4"

var (
	sharedFolder   string
	sessionPin     string
	uploadPin      string
	uploadsEnabled bool
	secretKey      string
	serverMutex    sync.Mutex
)

type FileInfo struct {
	Name  string
	Size  string
	IsDir bool
	Path  string
}

type Breadcrumb struct {
	Name   string
	Path   string
	IsLast bool
}

func main() {
	// Crash Logger Helper
	logError := func(err interface{}) {
		f, fileErr := os.OpenFile("crash.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if fileErr == nil {
			defer f.Close()
			fmt.Fprintf(f, "[%s] ERROR: %v\nStack: %s\n", time.Now().Format("2006-01-02 15:04:05"), err, string(debugStack()))
		}
		walk.MsgBox(nil, "Critical Error", fmt.Sprintf("A critical error occurred: %v\nPlease check crash.log for details.", err), walk.MsgBoxIconError)
		os.Exit(1)
	}

	// Configuration
	sharedFolder = "" 

	// Dynamic PIN Generation
	sessionPin = generateNewPIN()
	uploadPin = generateNewPIN() 

	// Secret Key for Cookies
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		logError("Failed to generate secret key: " + err.Error())
	}
	secretKey = fmt.Sprintf("%x", key)

	// Router
	mux := http.NewServeMux()
	mux.HandleFunc("/", authMiddleware(handleIndex))
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/logout", handleLogout)
	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/upload", handleUpload)
	mux.HandleFunc("/download-bulk", authMiddleware(handleBulkDownload))
	mux.HandleFunc("/download/", authMiddleware(handleDownload))
	mux.HandleFunc("/static/", handleStatic)

	port := "5000"
	addr := fmt.Sprintf("0.0.0.0:%s", port)

	// Server State
	var (
		srv           *http.Server
		serverRunning = false

		// GUI Widgets
		mw            *walk.Dialog
		folderEdit    *walk.LineEdit
		statusLabel   *walk.Label
		toggleBtn     *walk.PushButton
		portEdit      *walk.LineEdit
		pinEdit       *walk.LineEdit
		localUrlEdit  *walk.LineEdit
		networkIPEdit *walk.LineEdit
		statusLight   *walk.Label
		uploadChecked *walk.CheckBox
		uploadPinEdit *walk.LineEdit
	)

	// Helper to start server
	startServer := func() {
		serverMutex.Lock()
		defer serverMutex.Unlock()

		if srv != nil {
			return
		}

		if folderEdit != nil {
			sharedFolder = folderEdit.Text()
		}

		if sharedFolder == "" {
			walk.MsgBox(mw, "No Folder Selected", "Please select a folder to share before starting the server.", walk.MsgBoxIconWarning)
			return
		}

		if _, err := os.Stat(sharedFolder); err != nil {
			walk.MsgBox(mw, "Invalid Folder", "The selected folder does not exist or is inaccessible.", walk.MsgBoxIconError)
			return
		}

		currentPort := "5000"
		if portEdit != nil {
			portText := portEdit.Text()
			if portText == "" {
				currentPort = "5000"
			} else {
				p, err := strconv.Atoi(portText)
				if err != nil || p < 0 || p > 65535 {
					walk.MsgBox(mw, "Invalid Port", "Please enter a valid port number between 0 and 65535.", walk.MsgBoxIconError)
					return
				}
				currentPort = portText
			}
		}

		if pinEdit != nil {
			pinEdit.SetText(sessionPin)
		}
		if uploadPinEdit != nil {
			uploadPinEdit.SetText(uploadPin)
		}

		selectedInterface := ""
		if networkIPEdit != nil {
			selectedInterface = networkIPEdit.Text()
		}

		if selectedInterface == "" {
			selectedInterface = "0.0.0.0"
		}

		addr = fmt.Sprintf("0.0.0.0:%s", currentPort)

		srv = &http.Server{
			Addr:    addr,
			Handler: mux,
		}

		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Server start error: %v", err)
			}
		}()

		serverRunning = true
		if statusLight != nil {
			statusLight.SetTextColor(walk.RGB(0, 200, 80))
		}
		if statusLabel != nil {
			statusLabel.SetText("RUNNING")
			statusLabel.SetTextColor(walk.RGB(0, 200, 80))
		}
		if toggleBtn != nil {
			toggleBtn.SetText("Stop Server")
		}

		if localUrlEdit != nil {
			localUrlEdit.SetText(fmt.Sprintf("http://localhost:%s", currentPort))
		}
		if networkIPEdit != nil {
			ip := getBestIP()
			networkIPEdit.SetText(fmt.Sprintf("http://%s:%s", ip, currentPort))
		}
	}

	stopServer := func() {
		serverMutex.Lock()
		defer serverMutex.Unlock()

		if srv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				log.Printf("Server shutdown error: %v", err)
			}
			srv = nil
		}
		serverRunning = false
		if statusLight != nil {
			statusLight.SetTextColor(walk.RGB(230, 0, 0))
		}
		if statusLabel != nil {
			statusLabel.SetText("STOPPED")
			statusLabel.SetTextColor(walk.RGB(230, 0, 0))
		}
		if toggleBtn != nil {
			toggleBtn.SetText("Start Server")
		}
	}

	currentIP := getBestIP()
	localURL := fmt.Sprintf("http://localhost:%s", port)

	// --- ICON KODLARI TAMAMEN KALDIRILDI ---

	if _, err := (Dialog{
		AssignTo:  &mw,
		Title:     "Linera Server Manager " + AppVersion,
		// Icon:  Kaldırıldı (Varsayılan sistem ikonu kullanılacak)
		MinSize:   Size{Width: 450, Height: 500},
		FixedSize: true,
		Layout:    VBox{},
		Children: []Widget{
			// Status Section
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 5},
				Children: []Widget{
					Label{
						AssignTo:  &statusLight,
						Text:      "●",
						Font:      Font{PointSize: 12, Bold: true},
						TextColor: walk.RGB(230, 0, 0),
					},
					Label{
						Text: "Server Status:",
						Font: Font{PointSize: 10, Bold: true},
					},
					Label{
						AssignTo:  &statusLabel,
						Text:      "STOPPED",
						Font:      Font{PointSize: 10, Bold: true},
						TextColor: walk.RGB(230, 0, 0),
					},
					HSpacer{},
					PushButton{
						Text:    "?",
						MaxSize: Size{Width: 30},
						OnClicked: func() {
							walk.MsgBox(mw, "About Linera", "Linera Server Manager "+AppVersion+"\n\nAuthor: faruk-guler\nWebsite: www.farukguler.com", walk.MsgBoxIconInformation)
						},
					},
				},
			},
			PushButton{
				AssignTo: &toggleBtn,
				Text:     "Start Server",
				OnClicked: func() {
					if serverRunning {
						stopServer()
					} else {
						startServer()
					}
				},
			},
			VSpacer{Size: 10},

			Label{
				Text: "Server Configuration:",
				Font: Font{PointSize: 9, Bold: true},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{Text: "Port:"},
					LineEdit{
						AssignTo: &portEdit,
						Text:     "5000",
						MaxSize:  Size{Width: 60},
					},
					HSpacer{Size: 30},
					LinkLabel{
						Text: "Security PIN (<a id=\"new\">New</a>):",
						Font: Font{Bold: true},
						OnLinkActivated: func(link *walk.LinkLabelLink) {
							sessionPin = generateNewPIN()
							pinEdit.SetText(sessionPin)
						},
					},
					LineEdit{
						AssignTo: &pinEdit,
						Text:     sessionPin,
						MaxSize:  Size{Width: 120},
						Font:     Font{PointSize: 11, Bold: true},
						ReadOnly: true,
					},
				},
			},
			VSpacer{Size: 10},

			Label{
				Text: "Shared Folder:",
				Font: Font{PointSize: 9, Bold: true},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					LineEdit{
						AssignTo: &folderEdit,
						Text:     sharedFolder,
						ReadOnly: true,
					},
					PushButton{
						Text: "Browse...",
						OnClicked: func() {
							dlg := new(walk.FileDialog)
							dlg.FilePath = sharedFolder
							dlg.Title = "Select Folder to Share"
							if ok, _ := dlg.ShowBrowseFolder(mw); ok {
								sharedFolder = dlg.FilePath
								folderEdit.SetText(sharedFolder)
							}
						},
					},
				},
			},
			VSpacer{Size: 10},

			Label{
				Text: "Upload (Inbox) Settings:",
				Font: Font{PointSize: 9, Bold: true},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					CheckBox{
						AssignTo: &uploadChecked,
						Text:     "Enable Remote Uploads",
						Checked:  uploadsEnabled,
						OnCheckedChanged: func() {
							uploadsEnabled = uploadChecked.Checked()
						},
					},
					HSpacer{Size: 20},
					LinkLabel{
						Text: "Upload PIN (<a id=\"new\">New</a>):",
						OnLinkActivated: func(link *walk.LinkLabelLink) {
							uploadPin = generateNewPIN()
							uploadPinEdit.SetText(uploadPin)
						},
					},
					LineEdit{
						AssignTo: &uploadPinEdit,
						Text:     uploadPin,
						MaxSize:  Size{Width: 80},
						Font:     Font{Bold: true},
						ReadOnly: true,
					},
				},
			},
			VSpacer{Size: 10},

			Label{
				Text: "Local Address:",
				Font: Font{PointSize: 9},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					LineEdit{
						AssignTo: &localUrlEdit,
						Text:     localURL,
						ReadOnly: true,
					},
					PushButton{
						Text: "Copy",
						OnClicked: func() {
							walk.Clipboard().SetText(localUrlEdit.Text())
						},
					},
					PushButton{
						Text: "Open",
						OnClicked: func() {
							openBrowser(localUrlEdit.Text())
						},
					},
				},
			},

			Label{
				Text: "Network Address:",
				Font: Font{PointSize: 9},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					LineEdit{
						AssignTo: &networkIPEdit,
						Text:     fmt.Sprintf("http://%s:%s", currentIP, port),
						ReadOnly: true,
					},
					PushButton{
						Text: "Copy",
						OnClicked: func() {
							walk.Clipboard().SetText(networkIPEdit.Text())
						},
					},
					PushButton{
						Text: "📱 QR",
						OnClicked: func() {
							link := networkIPEdit.Text()
							if link == "" || strings.Contains(link, "127.0.0.1") {
								walk.MsgBox(mw, "Error", "No valid network address found.", walk.MsgBoxIconError)
								return
							}

							content, err := qrcode.Encode(link, qrcode.Medium, 256)
							if err != nil {
								walk.MsgBox(mw, "Error", "Failed to generate QR code.", walk.MsgBoxIconError)
								return
							}

							qrPath := filepath.Join(os.TempDir(), "linera_qr.png")
							if err := os.WriteFile(qrPath, content, 0644); err != nil {
								walk.MsgBox(mw, "Error", "Failed to save QR code.", walk.MsgBoxIconError)
								return
							}

							var dlg *walk.Dialog
							var iv *walk.ImageView

							img, _ := walk.NewImageFromFile(qrPath)

							Dialog{
								AssignTo: &dlg,
								Title:    "Scan to Connect",
								MinSize:  Size{Width: 300, Height: 350},
								Layout:   VBox{},
								Children: []Widget{
									ImageView{
										AssignTo: &iv,
										Image:    img,
										Mode:     ImageViewModeCenter,
									},
									Label{
										Text:          link,
										TextAlignment: AlignCenter,
										Font:          Font{PointSize: 10, Bold: true},
									},
									PushButton{
										Text: "Close",
										OnClicked: func() {
											dlg.Close(0)
										},
									},
								},
							}.Run(mw)
						},
					},
				},
			},
			VSpacer{Size: 10},
		},
	}).Run(nil); err != nil {
		logError(err)
	}
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		cookie, err := r.Cookie("session_token")
		if err != nil || cookie.Value != secretKey {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		pin := r.FormValue("pin")
		if pin == sessionPin {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    secretKey,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		time.Sleep(1 * time.Second)
		tmpl, err := template.ParseFS(assets, "assets/login.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		tmpl.Execute(w, map[string]interface{}{
			"Error":   "Invalid PIN",
			"Version": AppVersion,
		})
		return
	}

	tmpl, err := template.ParseFS(assets, "assets/login.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]interface{}{
		"Version": AppVersion,
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	subPath := r.URL.Query().Get("path")
	cleanPath := filepath.Clean(subPath)
	if strings.Contains(cleanPath, "..") || strings.HasPrefix(cleanPath, "\\") || strings.HasPrefix(cleanPath, "/") {
		cleanPath = ""
	}

	fullPath := filepath.Join(sharedFolder, cleanPath)

	relCheck, err := filepath.Rel(sharedFolder, fullPath)
	if err != nil || strings.HasPrefix(relCheck, "..") {
		fullPath = sharedFolder
		cleanPath = ""
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		fullPath = sharedFolder
		cleanPath = ""
		entries, _ = os.ReadDir(fullPath)
	}

	var files []FileInfo
	for _, e := range entries {
		info, _ := e.Info()
		size := formatSize(info.Size())
		if e.IsDir() {
			size = "-"
		}

		reqPath := e.Name()
		if cleanPath != "" {
			reqPath = filepath.Join(cleanPath, e.Name())
		}
		reqPath = strings.ReplaceAll(reqPath, "\\", "/")

		files = append(files, FileInfo{
			Name:  e.Name(),
			Size:  size,
			IsDir: e.IsDir(),
			Path:  reqPath,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	breadcrumbs := []Breadcrumb{{Name: "Home", Path: "", IsLast: false}}
	if cleanPath != "" {
		parts := strings.Split(strings.ReplaceAll(cleanPath, "\\", "/"), "/")
		current := ""
		for _, p := range parts {
			if current == "" {
				current = p
			} else {
				current = current + "/" + p
			}
			breadcrumbs = append(breadcrumbs, Breadcrumb{Name: p, Path: current, IsLast: false})
		}
	}
	if len(breadcrumbs) > 0 {
		breadcrumbs[len(breadcrumbs)-1].IsLast = true
	}

	parentPath := ""
	if cleanPath != "" {
		parentPath = filepath.Dir(cleanPath)
		if parentPath == "." || parentPath == "\\" || parentPath == "/" {
			parentPath = ""
		}
	}
	parentPath = strings.ReplaceAll(parentPath, "\\", "/")

	tmpl, err := template.ParseFS(assets, "assets/index.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	currentPathForTemplate := strings.ReplaceAll(cleanPath, "\\", "/")

	tmpl.Execute(w, map[string]interface{}{
		"files":        files,
		"current_path": currentPathForTemplate,
		"breadcrumbs":  breadcrumbs,
		"parent_path":  parentPath,
		"Version":      AppVersion,
	})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/download/")
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		http.Error(w, "Access denied: Path traversal detected", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(sharedFolder, cleanPath)

	rel, err := filepath.Rel(sharedFolder, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, "Access denied: Path out of bounds", http.StatusForbidden)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if info.IsDir() {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", filepath.Base(fullPath)))

		zw := zip.NewWriter(w)
		defer zw.Close()

		filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(fullPath, path)
			if err != nil {
				return err
			}

			header.Name = filepath.ToSlash(relPath)

			if info.IsDir() {
				header.Name += "/"
			} else {
				header.Method = zip.Deflate
			}

			writer, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			return err
		})
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(fullPath))
	http.ServeFile(w, r, fullPath)
}

func handleBulkDownload(w http.ResponseWriter, r *http.Request) {

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathsStr := r.FormValue("paths")
	if pathsStr == "" {
		http.Error(w, "No paths selected", http.StatusBadRequest)
		return
	}

	paths := strings.Split(pathsStr, "|")
	if len(paths) == 0 {
		http.Error(w, "No paths selected", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"bulk_download.zip\"")

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, relPath := range paths {
		cleanRel := filepath.Clean(relPath)
		if strings.Contains(cleanRel, "..") {
			continue 
		}

		fullPath := filepath.Join(sharedFolder, cleanRel)

		relCheck, err := filepath.Rel(sharedFolder, fullPath)
		if err != nil || strings.HasPrefix(relCheck, "..") {
			continue 
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			continue 
		}

		if info.IsDir() {
			filepath.Walk(fullPath, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				header, err := zip.FileInfoHeader(info)
				if err != nil {
					return err
				}

				parentOfRequested := filepath.Dir(fullPath)
				zipRelPath, _ := filepath.Rel(parentOfRequested, p)

				header.Name = filepath.ToSlash(zipRelPath)
				if info.IsDir() {
					header.Name += "/"
				} else {
					header.Method = zip.Deflate
				}

				writer, err := zw.CreateHeader(header)
				if err != nil {
					return err
				}

				if !info.IsDir() {
					file, err := os.Open(p)
					if err != nil {
						return err
					}
					defer file.Close()
					io.Copy(writer, file)
				}
				return nil
			})
		} else {
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				continue
			}
			header.Name = filepath.Base(fullPath)
			header.Method = zip.Deflate

			writer, err := zw.CreateHeader(header)
			if err != nil {
				continue
			}

			file, err := os.Open(fullPath)
			if err != nil {
				continue
			}
			defer file.Close()
			io.Copy(writer, file)
		}
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {

	if !uploadsEnabled {
		http.Error(w, "Uploads are disabled", http.StatusForbidden)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pin := r.Header.Get("X-Upload-PIN")
	if pin == "" {
		pin = r.FormValue("pin")
	}

	if pin != uploadPin {
		http.Error(w, "Invalid Upload PIN", http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "Form parsing error", http.StatusBadRequest)
		return
	}

	relPath := r.FormValue("path")
	cleanRel := filepath.Clean(relPath)
	if strings.Contains(cleanRel, "..") {
		http.Error(w, "Invalid upload path", http.StatusBadRequest)
		return
	}

	targetDir := filepath.Join(sharedFolder, cleanRel)

	relCheck, err := filepath.Rel(sharedFolder, targetDir)
	if err != nil || strings.HasPrefix(relCheck, "..") {
		http.Error(w, "Access denied: Path out of bounds", http.StatusForbidden)
		return
	}

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		http.Error(w, "Target directory does not exist", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	safeName := filepath.Base(handler.Filename)
	dstPath := filepath.Join(targetDir, safeName)

	dst, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "Error creating destination file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "File uploaded successfully: %s", safeName)
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	content, err := assets.ReadFile("assets/" + path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ext := filepath.Ext(path)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		switch ext {
		case ".css":
			mimeType = "text/css"
		case ".js":
			mimeType = "application/javascript"
		case ".png":
			mimeType = "image/png"
		case ".jpg", ".jpeg":
			mimeType = "image/jpeg"
		case ".svg":
			mimeType = "image/svg+xml"
		default:
			mimeType = "application/octet-stream"
		}
	}
	w.Header().Set("Content-Type", mimeType)

	w.Write(content)
}

func getBestIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	bestIP := ""
	bestScore := -1

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip = ip.To4()
			if ip == nil {
				continue
			}

			ipStr := ip.String()
			isPrivate := strings.HasPrefix(ipStr, "192.168.") ||
				strings.HasPrefix(ipStr, "10.") ||
				(strings.HasPrefix(ipStr, "172.") && len(ipStr) >= 6 && ipStr[4] >= '1' && ipStr[4] <= '3')

			if !isPrivate {
				continue
			}

			score := 0
			if strings.HasPrefix(ipStr, "192.168.") {
				score = 100
			} else {
				score = 50
			}

			lowerName := strings.ToLower(iface.Name)
			if strings.Contains(lowerName, "wi-fi") || strings.Contains(lowerName, "ethernet") {
				score += 50
			}

			if strings.Contains(lowerName, "virtual") || strings.Contains(lowerName, "vbox") ||
				strings.Contains(lowerName, "vmware") || strings.Contains(lowerName, "docker") ||
				strings.Contains(lowerName, "vEthernet") {
				score -= 200 
			}

			if score > bestScore {
				bestScore = score
				bestIP = ipStr
			}
		}
	}

	if bestIP == "" {
		return "127.0.0.1"
	}

	return bestIP
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}
	if err != nil {
		log.Println(err)
	}
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
func debugStack() []byte {
	buf := make([]byte, 1024)
	n := runtime.Stack(buf, false)
	return buf[:n]
}

func generateNewPIN() string {
	pinBytes := make([]byte, 2)
	if _, err := rand.Read(pinBytes); err != nil {
		return fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
	}
	pinVal := binary.BigEndian.Uint16(pinBytes)
	return fmt.Sprintf("%04d", pinVal%10000)
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}