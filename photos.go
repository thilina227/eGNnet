package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Member photos are stored as files in <app-data>/photos, not as DB blobs.
// member.photo_path holds the bare filename (e.g. "<member-id>.jpg").

const maxPhotoBytes = 5 << 20 // 5 MiB

// allowed image content types -> file extension
var photoTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

func (s *Server) photoDir() string { return filepath.Join(s.dataDir, "photos") }

func (s *Server) memberPhotoPath(memberID string) (string, bool) {
	var p *string
	err := s.db.QueryRow(`SELECT photo_path FROM member WHERE id=? AND deleted_at IS NULL`, memberID).Scan(&p)
	if err != nil || p == nil || *p == "" {
		return "", false
	}
	return *p, true
}

// GET /api/members/{id}/photo — serve the stored image.
func (s *Server) handleMemberPhotoGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, ok := s.memberPhotoPath(id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "no photo"})
		return
	}
	full := filepath.Join(s.photoDir(), filepath.Base(name))
	f, err := os.Open(full)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no photo"})
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", contentTypeForExt(filepath.Ext(name)))
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, f)
}

// POST /api/members/{id}/photo — multipart upload (field "photo").
func (s *Server) handleMemberPhotoUpload(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")

	// confirm the member exists (and isn't deleted)
	var exists int
	s.db.QueryRow(`SELECT COUNT(*) FROM member WHERE id=? AND deleted_at IS NULL`, id).Scan(&exists)
	if exists == 0 {
		writeJSON(w, 404, map[string]string{"error": "member not found"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+1024)
	file, _, err := r.FormFile("photo")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	// sniff content type from the first 512 bytes
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	head = head[:n]
	ct := http.DetectContentType(head)
	ext, ok := photoTypes[ct]
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "unsupported image type (use JPEG, PNG or WebP)"})
		return
	}

	if err := os.MkdirAll(s.photoDir(), 0o755); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	// remove any previous file for this member (possibly a different extension)
	s.removeMemberPhotoFile(id)

	name := id + ext
	full := filepath.Join(s.photoDir(), name)
	out, err := os.Create(full)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	written := int64(len(head))
	if _, err := out.Write(head); err != nil {
		out.Close()
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	nn, err := io.Copy(out, file)
	written += nn
	out.Close()
	if err != nil || written > maxPhotoBytes {
		_ = os.Remove(full)
		writeJSON(w, 400, map[string]string{"error": "image too large (max 5 MB)"})
		return
	}

	if _, err := s.db.Exec(`UPDATE member SET photo_path=?, updated_at=? WHERE id=?`, name, nowISO(), id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "member", id, "update", map[string]string{"photo": "set"})
	if s.log != nil {
		s.log.write("info", "admin", sess.userID, sess.username, "member_photo_set", "member "+id)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "photo_path": name})
}

// DELETE /api/members/{id}/photo
func (s *Server) handleMemberPhotoDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	s.removeMemberPhotoFile(id)
	s.db.Exec(`UPDATE member SET photo_path=NULL, updated_at=? WHERE id=?`, nowISO(), id)
	s.db.audit(sess.userID, "member", id, "update", map[string]string{"photo": "removed"})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// removeMemberPhotoFile deletes the current photo file for a member, if any.
func (s *Server) removeMemberPhotoFile(memberID string) {
	if name, ok := s.memberPhotoPath(memberID); ok {
		_ = os.Remove(filepath.Join(s.photoDir(), filepath.Base(name)))
	}
}

func contentTypeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
