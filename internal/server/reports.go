package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"mkvcheck/internal/findings"
	"mkvcheck/internal/report"
	"mkvcheck/internal/store"
)

const (
	proofHeader         = "X-Altcha"
	maxProofHeaderBytes = 4096
	markdownSuffix      = ".md"
	bbcodeSuffix        = ".bb"
)

type createdReport struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Proof of work is verified after parsing so a rejected request never burns a solution.
func (h handlers) createReport(writer http.ResponseWriter, request *http.Request) {
	if !h.guard.AllowRequest(request) {
		writeError(writer, http.StatusTooManyRequests, "rate_limited")
		return
	}
	proof := request.Header.Get(proofHeader)
	if len(proof) > maxProofHeaderBytes {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	parsed, status, code := readFindings(writer, request)
	if code != "" {
		writeError(writer, status, code)
		return
	}
	if proof == "" || !h.guard.VerifySolution(proof) {
		writeError(writer, http.StatusForbidden, "proof_of_work")
		return
	}
	saved, err := h.reports.Save(parsed)
	if errors.Is(err, store.ErrFull) {
		writeError(writer, http.StatusServiceUnavailable, "storage_full")
		return
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(writer, http.StatusCreated, createdReport{ID: saved.ID, URL: "/r/" + saved.ID, ExpiresAt: saved.ExpiresAt})
}

func readFindings(writer http.ResponseWriter, request *http.Request) (findings.Findings, int, string) {
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, findings.MaxBodyBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return findings.Findings{}, http.StatusRequestEntityTooLarge, "too_large"
	}
	if err != nil {
		return findings.Findings{}, http.StatusBadRequest, "invalid_json"
	}
	parsed, err := findings.Parse(body)
	if errors.Is(err, findings.ErrTooLarge) {
		return findings.Findings{}, http.StatusRequestEntityTooLarge, "too_large"
	}
	if err != nil {
		return findings.Findings{}, http.StatusBadRequest, "invalid_json"
	}
	return parsed, 0, ""
}

func (h handlers) showReport(writer http.ResponseWriter, request *http.Request) {
	id, format := splitFormat(request.PathValue("id"))
	if format == "" {
		http.NotFound(writer, request)
		return
	}
	stored, err := h.reports.Get(id)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Robots-Tag", "noindex")
	renderReport(writer, stored, format)
}

func splitFormat(segment string) (string, string) {
	if id, found := strings.CutSuffix(segment, markdownSuffix); found {
		return id, markdownSuffix
	}
	if id, found := strings.CutSuffix(segment, bbcodeSuffix); found {
		return id, bbcodeSuffix
	}
	if strings.Contains(segment, ".") {
		return "", ""
	}
	return segment, "html"
}

func renderReport(writer http.ResponseWriter, stored report.Report, format string) {
	switch format {
	case markdownSuffix:
		writeText(writer, "text/markdown; charset=utf-8", report.Markdown(stored))
	case bbcodeSuffix:
		writeText(writer, "text/plain; charset=utf-8", report.BBCode(stored))
	default:
		var page bytes.Buffer
		if err := report.HTML(&page, stored); err != nil {
			http.Error(writer, "internal error", http.StatusInternalServerError)
			return
		}
		writeText(writer, "text/html; charset=utf-8", page.String())
	}
}

func writeText(writer http.ResponseWriter, contentType, body string) {
	writer.Header().Set("Content-Type", contentType)
	_, _ = io.WriteString(writer, body)
}
