package relayd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
)

// Handler is the control API of a relay (relayctl's documentation lists it).
func (r *Relay) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, r.Status(), nil)
	})
	mux.HandleFunc("PUT /v1/config", func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 16<<20))
		if err != nil {
			reply(w, nil, fmt.Errorf("%w: %w", errInvalid, err))
			return
		}
		changed, digest, err := r.Apply(body)
		if err != nil {
			reply(w, nil, err)
			return
		}
		reply(w, relayctl.ApplyReply{Changed: changed, Digest: digest}, nil)
	})
	mux.HandleFunc("GET /v1/observe", func(w http.ResponseWriter, req *http.Request) {
		reply(w, r.Observe(req.URL.Query().Get("drain") == "1"), nil)
	})
	mux.HandleFunc("PUT /v1/rotation", func(w http.ResponseWriter, req *http.Request) {
		var rot relayctl.Rotation
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&rot); err != nil {
			reply(w, nil, fmt.Errorf("%w: %w", errInvalid, err))
			return
		}
		reply(w, struct{}{}, r.SetRotation(rot))
	})
	mux.HandleFunc("POST /v1/credentials", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, struct{}{}, r.ReloadCredentials())
	})
	return mux
}

func reply(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		e := relayctl.Error{Code: relayctl.CodeInternal, Message: err.Error()}
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrConflict):
			e.Code, status = relayctl.CodeConflict, http.StatusConflict
		case errors.Is(err, errNotFound):
			e.Code, status = relayctl.CodeNotFound, http.StatusNotFound
		case errors.Is(err, errInvalid):
			e.Code, status = relayctl.CodeInvalid, http.StatusBadRequest
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(e)
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// Serve serves the control API on a unix socket at path until ctx ends. The
// socket's permissions are its only key: mode 0660, and the unit's UMask and
// RuntimeDirectory keep it to the relay's user and group, which the Agent
// belongs to. It returns when the server has stopped.
func (r *Relay) Serve(ctx context.Context, path string) error {
	_ = os.Remove(path) // a stale socket of a crashed predecessor
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o660); err != nil { // #nosec G302 -- group access to the control socket is the design: its permissions are its only key
		_ = ln.Close()
		return err
	}
	srv := &http.Server{Handler: r.Handler(), ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
		_ = os.Remove(path)
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
