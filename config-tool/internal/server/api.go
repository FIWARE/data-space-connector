package server

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/FIWARE/data-space-connector/config-tool/internal/cluster"
	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	"github.com/FIWARE/data-space-connector/config-tool/internal/validate"
	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
)

const (
	// maxBodyBytes caps request bodies; a full values.yaml is a few hundred KiB.
	maxBodyBytes    = 8 << 20
	contentTypeJSON = "application/json"
	eventReady      = "ready"
	// queryKubeconfig names the explicit kubeconfig path; empty means clientcmd's default rules.
	queryKubeconfig = "kubeconfig"
	// queryName is the uploaded file's name; only its base name is kept, for the report.
	queryName = "name"
	// queryRoot is where the chart's values sit in the file, in path syntax; absent means detect.
	queryRoot = "root"
	// defaultLoadName stands in for a load without a usable name.
	defaultLoadName = "values.yaml"
	// pathSeparators covers browser-supplied names from both POSIX and Windows.
	pathSeparators = `/\`
)

var errTrailingData = errors.New("unexpected data after the JSON value")

type errorBody struct {
	Error  string `json:"error"`
	Ticket string `json:"ticket,omitempty"`
	// Fields lists the structural errors of a refused profile.
	Fields []profile.FieldError `json:"fields,omitempty"`
}

// Handler is the full HTTP handler: guards, API routes and the UI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/model", s.handleModel)
	mux.HandleFunc("GET /api/cluster/contexts", handleContexts)
	mux.HandleFunc("PUT /api/profile", s.handleSaveProfile)
	mux.HandleFunc("POST /api/profile/open", s.handleOpenProfile)
	mux.HandleFunc("GET /api/profile/recent", s.handleRecentProfiles)
	mux.HandleFunc("GET /api/roles", s.handleRoles)
	mux.HandleFunc("POST /api/preset", s.handlePreset)
	mux.HandleFunc("POST /api/features", s.handleFeatures)
	mux.HandleFunc("POST /api/load", s.handleLoad)
	mux.HandleFunc("GET /api/trial", s.handleTrial)
	mux.HandleFunc("POST /api/trial/write", s.handleTrialWrite)
	mux.HandleFunc("POST /api/render", s.handleRender)
	mux.HandleFunc("GET /api/foundations", s.handleFoundations)
	mux.HandleFunc("GET /api/infrastructure", s.handleInfrastructure)
	mux.HandleFunc("POST /api/dataspace/check", s.handleDataspaceCheck)
	mux.HandleFunc("POST /api/dataspace/import", s.handleDataspaceImport)
	mux.HandleFunc("POST /api/validate", handleValidate)
	mux.HandleFunc("GET "+pathWS, s.handleWebSocket)
	mux.HandleFunc(pathAPI, func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint", "")
	})
	mux.Handle(pathRoot, staticHandler(s.ui))
	return s.guard(mux)
}

// modelBody is the chart and version choice of the Target stage; the questions arrive with later epics.
type modelBody struct {
	Charts []modelChart `json:"charts"`
}

// modelChart carries its own versions, so the Version dropdown only offers the selected chart's.
type modelChart struct {
	Name     string         `json:"name"`
	Versions []modelVersion `json:"versions"`
}

type modelVersion struct {
	Version string `json:"version"`
	Latest  bool   `json:"latest"`
}

func (s *Server) handleModel(w http.ResponseWriter, _ *http.Request) {
	bundles, err := model.Versions(s.models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	charts := model.Charts(bundles)
	body := modelBody{Charts: make([]modelChart, 0, len(charts))}
	for _, c := range charts {
		chart := modelChart{Name: c.Name, Versions: make([]modelVersion, 0, len(c.Versions))}
		for _, b := range c.Versions {
			chart.Versions = append(chart.Versions, modelVersion{Version: b.Version, Latest: b.Latest})
		}
		body.Charts = append(body.Charts, chart)
	}
	writeJSON(w, body)
}

// handleContexts lists kube context names; cluster.Kubeconfig carries no credentials or server URLs.
func handleContexts(w http.ResponseWriter, r *http.Request) {
	kubeconfig, err := cluster.Contexts(r.URL.Query().Get(queryKubeconfig))
	switch {
	case errors.Is(err, cluster.ErrKubeconfigNotFound):
		writeError(w, http.StatusNotFound, err.Error(), "")
	case err != nil:
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
	default:
		writeJSON(w, kubeconfig)
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

// handleLoad classifies the uploaded values.yaml, keeps it as the loaded slot and returns its report.
func (s *Server) handleLoad(w http.ResponseWriter, r *http.Request) {
	var opts load.Options
	if q := r.URL.Query(); q.Has(queryRoot) {
		root, err := roundtrip.ParsePath(q.Get(queryRoot))
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
			return
		}
		opts.Root = &root
	}
	src, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, bodyErrorStatus(err), err.Error(), "")
		return
	}
	bundles, err := model.Versions(s.models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	res, err := load.Load(src, baseName(r.URL.Query().Get(queryName)), bundles, opts)
	switch {
	case errors.Is(err, load.ErrNoBundle):
		writeError(w, http.StatusInternalServerError, err.Error(), "")
	case err != nil:
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
	default:
		s.loadMu.Lock()
		s.loaded = res
		s.resetRender()
		s.loadMu.Unlock()
		writeJSON(w, res.Report)
	}
}

// baseName drops any directory part of a browser-supplied file name.
func baseName(name string) string {
	name = name[strings.LastIndexAny(name, pathSeparators)+1:]
	if name == "" || name == "." || name == ".." {
		return defaultLoadName
	}
	return name
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set(headerContentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg, ticket string) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: msg, Ticket: ticket})
}

// writeFieldErrors refuses a structurally invalid profile with 422 and its field errors.
func writeFieldErrors(w http.ResponseWriter, errs []profile.FieldError) {
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(errorBody{Error: "invalid profile: " + strings.Join(msgs, "; "), Fields: errs})
}
