// internal/dashboard/stubs.go
// Stub handlers — replaced by real implementations in subsequent tasks.
package dashboard

import "net/http"

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
