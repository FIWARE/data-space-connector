package server

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	"github.com/FIWARE/data-space-connector/config-tool/internal/validate"
	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
)

const (
	// maxBodyBytes caps request bodies; a full values.yaml is a few hundred KiB.
	maxBodyBytes    = 8 << 20
	contentTypeJSON = "application/json"
	contentTypeYAML = "application/yaml"
	eventReady      = "ready"

	// The tickets that replace a stub, reported with its 501.
	ticketModel   = "CT-1"
	ticketRender  = "CT-2"
	ticketLoad    = "CT-2"
	ticketProfile = "CT-4"
)

var errTrailingData = errors.New("unexpected data after the JSON value")

type errorBody struct {
	Error  string `json:"error"`
	Ticket string `json:"ticket,omitempty"`
}

// Handler is the full HTTP handler: guards, API routes and the UI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/model", notImplemented("the question model is not available yet", ticketModel))
	mux.Handle("GET /api/profile", notImplemented("profiles are not available yet", ticketProfile))
	mux.Handle("PUT /api/profile", notImplemented("profiles are not available yet", ticketProfile))
	mux.Handle("POST /api/load", notImplemented("loading a values.yaml is not available yet", ticketLoad))
	mux.HandleFunc("POST /api/render", handleRender)
	mux.HandleFunc("POST /api/validate", handleValidate)
	mux.HandleFunc("GET "+pathWS, s.handleWebSocket)
	mux.HandleFunc(pathAPI, func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint", "")
	})
	mux.Handle(pathRoot, staticHandler(s.ui))
	return s.guard(mux)
}

func notImplemented(msg, ticket string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotImplemented, msg, ticket)
	})
}

func handleRender(w http.ResponseWriter, r *http.Request) {
	var p profile.Profile
	if err := decodeJSON(w, r, &p); err != nil {
		writeError(w, bodyErrorStatus(err), "invalid profile: "+err.Error(), "")
		return
	}
	values, err := render.Render(r.Context(), p)
	switch {
	case errors.Is(err, render.ErrNotImplemented):
		writeError(w, http.StatusNotImplemented, err.Error(), ticketRender)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error(), "")
	default:
		w.Header().Set(headerContentType, contentTypeYAML)
		_, _ = w.Write(values)
	}
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	values, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, bodyErrorStatus(err), err.Error(), "")
		return
	}
	err = validate.Validate(r.Context(), values)
	switch {
	case errors.Is(err, validate.ErrNotImplemented):
		writeError(w, http.StatusNotImplemented, err.Error(), "")
	case err != nil:
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// decodeJSON reads exactly one JSON value from the capped body into v; an empty body leaves v
// unchanged and anything after the value is an error.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	var extra json.RawMessage
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return nil
	case err != nil:
		return err
	default:
		return errTrailingData
	}
}

// bodyErrorStatus maps a request-body error to 413 when the body exceeded maxBodyBytes, else 400.
func bodyErrorStatus(err error) int {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// handleWebSocket sends the ready event, then forwards progress events until either side ends.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	port := strconv.Itoa(s.port)
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{net.JoinHostPort(loopbackHost, port), net.JoinHostPort(localhost, port)},
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(r.Context())
	events, unsubscribe := s.progress.Subscribe()
	defer unsubscribe()
	if err := wsjson.Write(ctx, conn, Event{Type: eventReady, Version: version.Version}); err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown.Done():
			conn.Close(websocket.StatusGoingAway, "dsc-config is shutting down")
			return
		case ev := <-events:
			if err := wsjson.Write(ctx, conn, ev); err != nil {
				return
			}
		}
	}
}

func writeError(w http.ResponseWriter, status int, msg, ticket string) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: msg, Ticket: ticket})
}
