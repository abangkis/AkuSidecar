package httpapi

import (
	_ "embed"
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/capturestartup"
)

//go:embed capture_startup.js
var captureStartupScript []byte

func (s *Server) captureStartupSample(w http.ResponseWriter, r *http.Request) error {
	if s.captureStartup == nil {
		return notFound("startup diagnostic")
	}
	t := s.splitCapture
	if !t.authorized(r) {
		return apiError{Status: http.StatusForbidden, Code: "capture_instance_mismatch", Message: "Capture sample rejected."}
	}
	var sample struct {
		Doc                                   int
		Phase                                 string
		Reload, ScriptStarted, WorkerReceived *bool
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256)
	if readJSON(r, &sample) != nil || (sample.Phase != "early" && sample.Phase != "late") ||
		sample.Doc < 1 || sample.Reload == nil || sample.ScriptStarted == nil || sample.WorkerReceived == nil {
		return badRequest("invalid startup sample")
	}
	s.captureStartup.Sample(capturestartup.Document{Number: sample.Doc, Loaded: true, Reload: *sample.Reload, ScriptStarted: *sample.ScriptStarted, WorkerReceived: *sample.WorkerReceived}, sample.Phase == "late")
	w.WriteHeader(http.StatusNoContent)
	return nil
}
