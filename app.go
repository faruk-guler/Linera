package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"embed"

	"github.com/skip2/go-qrcode"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// FileInfo representing a file or directory
type FileInfo struct {
	Name  string `json:"name"`  // Name of the file
	Size  string `json:"size"`  // Formatted size of the file
	IsDir bool   `json:"isDir"` // Whether the entry is a directory
	Path  string `json:"path"`  // Relative path of the file
}

// Breadcrumb for navigation
type Breadcrumb struct {
	Name   string `json:"name"`   // Name of the breadcrumb
	Path   string `json:"path"`   // Path for navigation
	IsLast bool   `json:"isLast"` // Whether this is the last item
}

// App struct
type App struct {
	ctx            context.Context
	sharedFolder   string
	sessionPin     string
	uploadPin      string
	uploadsEnabled bool
	secretKey      string
	serverRunning  bool
	port           string
	srv            *http.Server
	mutex          sync.Mutex
	Assets         embed.FS           // Embedded server assets
	Templates      *template.Template // Parsed HTML templates
	serverFS       fs.FS              // File system for server assets
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		port: "5000",
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.sessionPin = a.generateNewPIN()
	a.uploadPin = a.generateNewPIN()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Printf("Failed to generate secret key: %v", err)
	}
	a.secretKey = fmt.Sprintf("%x", key)

	// Create a sub-FS for the server assets (templates and static)
	sub, err := fs.Sub(a.Assets, "assets")
	if err != nil {
		log.Printf("Failed to create server sub-FS: %v", err)
		a.serverFS = a.Assets
	} else {
		a.serverFS = sub
	}
}

// SelectFolder opens a dialog to select a folder
func (a *App) SelectFolder() string {
	folder, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Folder to Share",
	})
	if err != nil {
		return ""
	}
	if folder != "" {
		a.sharedFolder = folder
	}
	return a.sharedFolder
}

// SetPort sets the server port
func (a *App) SetPort(p string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.port = p
}

// SetUploadEnabled toggles uploads
func (a *App) SetUploadEnabled(enabled bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.uploadsEnabled = enabled
}

// OpenSharedFolder opens the shared folder in the OS file explorer
func (a *App) OpenSharedFolder() {
	if a.sharedFolder != "" {
		runtime.BrowserOpenURL(a.ctx, a.sharedFolder)
	}
}

// OpenInBrowser opens the sharing link in the default browser
func (a *App) OpenInBrowser() {
	url := fmt.Sprintf("http://localhost:%s", a.port)
	runtime.BrowserOpenURL(a.ctx, url)
}

// RegenerateSessionPIN generates a new session PIN
func (a *App) RegenerateSessionPIN() string {
	a.sessionPin = a.generateNewPIN()
	return a.sessionPin
}

// RegenerateUploadPIN generates a new upload PIN
func (a *App) RegenerateUploadPIN() string {
	a.uploadPin = a.generateNewPIN()
	return a.uploadPin
}

// GetQRCode generates a QR code for the network address and returns it as a Base64 string
func (a *App) GetQRCode() (string, error) {
	link := fmt.Sprintf("http://%s:%s", a.getBestIP(), a.port)
	if link == "" || strings.Contains(link, "127.0.0.1") {
		return "", fmt.Errorf("no valid network address found")
	}

	png, err := qrcode.Encode(link, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(png), nil
}

// GetAboutInfo returns application version and author info
func (a *App) GetAboutInfo() map[string]string {
	return map[string]string{
		"version": "v7.3",
		"author":  "faruk-guler",
		"website": "www.farukguler.com",
	}
}

// ShowError shows a native error dialog
func (a *App) ShowError(title, message string) {
	runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:    runtime.ErrorDialog,
		Title:   title,
		Message: message,
	})
}

// ShowInfo shows a native info dialog
func (a *App) ShowInfo(title, message string) {
	runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:    runtime.InfoDialog,
		Title:   title,
		Message: message,
	})
}

// GetConfig returns the current configuration
func (a *App) GetConfig() map[string]interface{} {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return map[string]interface{}{
		"sharedFolder":   a.sharedFolder,
		"sessionPin":     a.sessionPin,
		"uploadPin":      a.uploadPin,
		"port":           a.port,
		"isRunning":      a.serverRunning,
		"uploadsEnabled": a.uploadsEnabled,
		"localAddr":      fmt.Sprintf("http://localhost:%s", a.port),
		"networkAddr":    fmt.Sprintf("http://%s:%s", a.getBestIP(), a.port),
	}
}

// StartServer starts the HTTP server
func (a *App) StartServer() string {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.serverRunning {
		return "ALREADY_RUNNING"
	}

	if a.sharedFolder == "" {
		return "NO_FOLDER"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.authMiddleware(a.handleIndex))
	mux.HandleFunc("/login", a.handleLogin)
	mux.HandleFunc("/logout", a.handleLogout)
	mux.HandleFunc("/ping", a.handlePing)
	mux.HandleFunc("/upload", a.handleUpload)
	mux.HandleFunc("/download-bulk", a.authMiddleware(a.handleBulkDownload))
	mux.HandleFunc("/download/", a.authMiddleware(a.handleDownload))
	mux.HandleFunc("/search", a.authMiddleware(a.handleSearch))
	mux.HandleFunc("/static/", a.handleStatic)

	a.srv = &http.Server{
		Addr:    "0.0.0.0:" + a.port,
		Handler: mux,
	}

	go func() {
		log.Printf("Server starting on http://%s:%s", a.getBestIP(), a.port)
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	a.serverRunning = true
	return "OK"
}

// StopServer stops the HTTP server
func (a *App) StopServer() {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.srv.Shutdown(ctx)
		a.srv = nil
	}
	a.serverRunning = false
}

// Private Helpers

func (a *App) generateNewPIN() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digits = "0123456789"
	pinBytes := make([]byte, 4)
	if _, err := rand.Read(pinBytes); err != nil {
		return "A1B2"
	}
	// Pattern: Letter-Digit-Letter-Digit
	return string([]byte{
		letters[int(pinBytes[0])%len(letters)],
		digits[int(pinBytes[1])%len(digits)],
		letters[int(pinBytes[2])%len(letters)],
		digits[int(pinBytes[3])%len(digits)],
	})
}

func (a *App) getBestIP() string {
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
				(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31)

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
				strings.Contains(lowerName, "vethernet") {
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

func (a *App) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil || cookie.Value != a.secretKey {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// Handlers (Adapted from main.go)
func (a *App) handlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		pin := r.FormValue("pin")
		if pin == a.sessionPin {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    a.secretKey,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		time.Sleep(1 * time.Second)
		a.renderTemplate(w, "login.html", map[string]interface{}{
			"Error": "Invalid PIN",
		})
		return
	}
	a.renderTemplate(w, "login.html", map[string]interface{}{
		"Version": "v7.3",
	})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
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

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	subPath := r.URL.Query().Get("path")
	cleanPath := filepath.Clean(subPath)
	if strings.Contains(cleanPath, "..") || strings.HasPrefix(cleanPath, "\\") || strings.HasPrefix(cleanPath, "/") {
		cleanPath = ""
	}

	fullPath := filepath.Join(a.sharedFolder, cleanPath)
	relCheck, err := filepath.Rel(a.sharedFolder, fullPath)
	if err != nil || strings.HasPrefix(relCheck, "..") {
		fullPath = a.sharedFolder
		cleanPath = ""
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		fullPath = a.sharedFolder
		cleanPath = ""
		entries, _ = os.ReadDir(fullPath)
	}

	var files []FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info == nil {
			continue // skip entries that can't be read
		}
		size := a.formatSize(info.Size())
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

	a.renderTemplate(w, "index.html", map[string]interface{}{
		"files":        files,
		"current_path": strings.ReplaceAll(cleanPath, "\\", "/"),
		"breadcrumbs":  breadcrumbs,
		"parent_path":  parentPath,
		"Version":      "v7.3",
	})
}

// addDirToZip walks a directory and adds all its contents to the zip writer.
// baseDir is the reference point for relative paths in the archive.
func addDirToZip(zw *zip.Writer, dirPath, baseDir string) {
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(baseDir, path)
		header.Name = filepath.ToSlash(relPath)
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		writer, _ := zw.CreateHeader(header)
		if !info.IsDir() {
			file, err := os.Open(path)
			if err == nil {
				io.Copy(writer, file)
				file.Close()
			}
		}
		return nil
	})
}

// addFileToZip adds a single file to the zip writer.
func addFileToZip(zw *zip.Writer, filePath string, info os.FileInfo) {
	header, _ := zip.FileInfoHeader(info)
	header.Name = filepath.Base(filePath)
	header.Method = zip.Deflate
	writer, _ := zw.CreateHeader(header)
	file, err := os.Open(filePath)
	if err == nil {
		io.Copy(writer, file)
		file.Close()
	}
}

func (a *App) handleDownload(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/download/")
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		http.Error(w, "Access denied", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(a.sharedFolder, cleanPath)
	rel, err := filepath.Rel(a.sharedFolder, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, "Access denied", http.StatusForbidden)
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
		addDirToZip(zw, fullPath, fullPath)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(fullPath)))
	http.ServeFile(w, r, fullPath)
}

func (a *App) handleBulkDownload(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"bulk_download.zip\"")

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, relPath := range paths {
		cleanRel := filepath.Clean(relPath)
		if strings.Contains(cleanRel, "..") {
			continue
		}
		fullPath := filepath.Join(a.sharedFolder, cleanRel)
		rel, err := filepath.Rel(a.sharedFolder, fullPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			addDirToZip(zw, fullPath, filepath.Dir(fullPath))
		} else {
			addFileToZip(zw, fullPath, info)
		}
	}
}

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	if !a.uploadsEnabled {
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

	if pin != a.uploadPin {
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
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	targetDir := filepath.Join(a.sharedFolder, cleanRel)
	relCheck, err := filepath.Rel(a.sharedFolder, targetDir)
	if err != nil || strings.HasPrefix(relCheck, "..") {
		http.Error(w, "Access denied", http.StatusForbidden)
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

	io.Copy(dst, file)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "File uploaded successfully: %s", safeName)
}

type searchResult struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  string `json:"size"`
	IsDir bool   `json:"is_dir"`
}

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		http.Error(w, "Query must be at least 2 characters", http.StatusBadRequest)
		return
	}

	// Check if query is a glob pattern
	isGlob := strings.ContainsAny(query, "*?")

	// Prepare for case-insensitive matching
	queryLower := strings.ToLower(query)
	queryUpper := strings.ToUpper(query) // For non-glob substring match

	results := make([]searchResult, 0)
	maxResults := 1000 // Increased limit for better coverage

	// Sentinel error for stopping the walk
	var errStopSearch = fmt.Errorf("search limit reached")
	limitReached := false

	filepath.WalkDir(a.sharedFolder, func(path string, d os.DirEntry, err error) error {
		if len(results) >= maxResults {
			limitReached = true
			return errStopSearch
		}
		if err != nil {
			return nil
		}

		name := d.Name()
		match := false

		if isGlob {
			// Case-insensitive glob matching
			matched, err := filepath.Match(queryLower, strings.ToLower(name))
			if err == nil && matched {
				match = true
			}
		} else {
			// Standard substring matching
			if strings.Contains(strings.ToUpper(name), queryUpper) {
				match = true
			}
		}

		if match {
			relPath, _ := filepath.Rel(a.sharedFolder, path)
			relPath = filepath.ToSlash(relPath)

			var sizeStr string
			if d.IsDir() {
				sizeStr = "-"
			} else {
				info, err := d.Info()
				if err != nil || info == nil {
					sizeStr = "?"
				} else {
					sizeStr = a.formatSize(info.Size())
				}
			}

			results = append(results, searchResult{
				Name:  name,
				Path:  relPath,
				Size:  sizeStr,
				IsDir: d.IsDir(),
			})
		}
		return nil
	})

	response := map[string]interface{}{
		"results":       results,
		"limit_reached": limitReached,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *App) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	data, err := fs.ReadFile(a.serverFS, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

func (a *App) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if a.Templates == nil {
		var err error
		a.Templates, err = template.ParseFS(a.serverFS, "*.html")
		if err != nil {
			log.Printf("Failed to parse templates: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := a.Templates.ExecuteTemplate(w, name, data)
	if err != nil {
		log.Printf("Template execution error (%s): %v", name, err)
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

func (a *App) formatSize(size int64) string {
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
