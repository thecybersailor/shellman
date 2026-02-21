package localapi

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxUploadImageSize = 16 * 1024 * 1024
const maxUploadImageFormBuffer = maxUploadImageSize + (1024 * 1024)

func (s *Server) registerSystemRoutes() {
	s.mux.HandleFunc("/api/v1/system/capabilities", s.handleSystemCapabilities)
	s.mux.HandleFunc("/api/v1/system/app-programs", s.handleSystemAppPrograms)
	s.mux.HandleFunc("/api/v1/system/select-directory", s.handleSelectDirectory)
	s.mux.HandleFunc("/api/v1/system/uploads/image", s.handleSystemImageUpload)
}

func (s *Server) handleSystemCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	respondOK(w, map[string]any{
		"directory_picker": s.deps.PickDirectory != nil,
	})
}

func (s *Server) handleSystemAppPrograms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.deps.AppProgramsStore == nil {
		respondError(w, http.StatusInternalServerError, "APP_PROGRAMS_UNAVAILABLE", "app programs store is unavailable")
		return
	}
	cfg, err := s.deps.AppProgramsStore.LoadOrInit()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "APP_PROGRAMS_LOAD_FAILED", err.Error())
		return
	}
	respondOK(w, cfg)
}

func (s *Server) handleSelectDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.deps.PickDirectory == nil {
		respondError(w, http.StatusNotImplemented, "PICK_DIRECTORY_UNAVAILABLE", "directory picker is unavailable")
		return
	}
	path, err := s.deps.PickDirectory()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PICK_DIRECTORY_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{"repo_root": path})
}

func (s *Server) handleSystemImageUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	if err := r.ParseMultipartForm(maxUploadImageFormBuffer); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_UPLOAD", "invalid multipart body")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_UPLOAD", "missing file")
		return
	}
	defer file.Close()

	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if contentType == "" && header.Filename != "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	if !strings.HasPrefix(contentType, "image/") {
		respondError(w, http.StatusBadRequest, "INVALID_UPLOAD", "only image files are supported")
		return
	}

	tmp, err := os.CreateTemp("", "shellman-img-*"+extensionForContentType(contentType))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "UPLOAD_WRITE_FAILED", err.Error())
		return
	}
	defer tmp.Close()

	size, err := io.CopyN(tmp, file, maxUploadImageSize+1)
	if err != nil && err != io.EOF {
		_ = os.Remove(tmp.Name())
		respondError(w, http.StatusInternalServerError, "UPLOAD_WRITE_FAILED", err.Error())
		return
	}
	if size == 0 {
		_ = os.Remove(tmp.Name())
		respondError(w, http.StatusBadRequest, "INVALID_UPLOAD", "empty file")
		return
	}
	if size > maxUploadImageSize {
		_ = os.Remove(tmp.Name())
		respondError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "image is too large")
		return
	}

	respondOK(w, map[string]any{
		"path": tmp.Name(),
		"size": size,
		"mime": contentType,
	})
}

func extensionForContentType(contentType string) string {
	switch strings.TrimSpace(strings.ToLower(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}
