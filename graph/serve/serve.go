// Package serve hands a dependency graph over HTTP.
//
// It takes a graph.Source rather than a graph, and extracts on every request.
// That is what lets the same handler serve a file read from disk and a container
// that is still running:
//
//	srv, err := serve.Listen("127.0.0.1:0", container)
//	fmt.Println(srv.URL())
//	err = srv.Serve()
//
// A running container is already a graph.Source, so watching one change as it is
// wired needs nothing from this package beyond a page that asks again.
//
// It lives apart from graph because it draws the graph to answer a request, and
// drawing means graph/html. No godi binary should carry that unless it asked for
// it, and importing this package is how you ask.
package serve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/html"
	"github.com/michalkurzeja/godi/v2/graph/json"
	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// Handler serves the graph of src: the page at /, and the graph itself at
// /graph.json for anything that would rather read the model.
func Handler(src graph.Source, opts ...Option) http.Handler {
	cfg := config{enc: html.New()}
	for _, opt := range opts {
		opt(&cfg)
	}

	h := &handler{src: src, cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.page)
	mux.HandleFunc("GET /graph.json", h.model)
	// A live preview belongs here, as GET /events, with the page listening on
	// it. Extracting per request is already what it would need.
	return mux
}

type handler struct {
	src graph.Source
	cfg config
}

func (h *handler) page(w http.ResponseWriter, _ *http.Request) {
	h.write(w, h.cfg.enc)
}

func (h *handler) model(w http.ResponseWriter, _ *http.Request) {
	h.write(w, json.New())
}

// write encodes into a buffer first. A page cut off halfway still arrives under
// a 200, which tells the reader nothing is wrong when something is.
func (h *handler) write(w http.ResponseWriter, enc graph.Encoder) {
	g, err := graph.Extract(h.src, h.cfg.extract...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	err = g.Encode(&buf, enc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", enc.Format().MediaType)
	w.Header().Set("Cache-Control", "no-store") // The next request may say something else.
	_, _ = buf.WriteTo(w)
}

// Server is bound to its port before it serves anything, which is what you need
// when the port was chosen for you.
type Server struct {
	srv *http.Server
	ln  net.Listener
}

// Listen binds addr and prepares to serve the graph of src. Pass a zero port to
// be given one; Addr and URL say which.
func Listen(addr string, src graph.Source, opts ...Option) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errorsx.Wrapf(err, "serve: listening on %s", addr)
	}

	return &Server{
		srv: &http.Server{
			Handler:           Handler(src, opts...),
			ReadHeaderTimeout: 10 * time.Second,
		},
		ln: ln,
	}, nil
}

// Addr is the address the server bound to.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// URL is the address to open. A server listening on every interface is reported as
// the loopback one, which is what a browser on this machine wants.
func (s *Server) URL() string {
	addr, ok := s.ln.Addr().(*net.TCPAddr)
	if !ok {
		return "http://" + s.ln.Addr().String()
	}

	host := addr.IP.String()
	if addr.IP.IsUnspecified() {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, fmt.Sprint(addr.Port)))
}

// Serve serves until Shutdown stops it, which it reports as no error at all.
func (s *Server) Serve() error {
	err := s.srv.Serve(s.ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops the server, letting requests in flight finish.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
