package act

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

var dockerVersion = regexp.MustCompile(`^/v[0-9]+\.[0-9]+(/.*)$`)

// dockerProxy applies ownership to every container act creates, including those
// created by plans loaded internally for reusable workflows.
func dockerProxy(cli client.APIClient, id string) http.Handler {
	run := rand.Text()
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return cli.Dialer()(ctx)
	}}
	proxy := &httputil.ReverseProxy{
		Rewrite:   func(r *httputil.ProxyRequest) { r.Out.URL.Scheme, r.Out.URL.Host = "http", "docker" },
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "Docker connection failed", http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if match := dockerVersion.FindStringSubmatch(path); match != nil {
			path = match[1]
		}
		switch {
		case r.Method == http.MethodPost && path == "/containers/create":
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
				http.Error(w, "invalid container configuration", http.StatusBadRequest)
				return
			}
			labels := map[string]string{}
			if raw := body["Labels"]; len(raw) > 0 && string(raw) != "null" {
				if err := json.Unmarshal(raw, &labels); err != nil || labels == nil {
					http.Error(w, "invalid container labels", http.StatusBadRequest)
					return
				}
			}
			labels[projectLabel] = id
			body["Labels"], _ = json.Marshal(labels)
			host, err := scopeJobVolumes(r.Context(), cli, id, run, body["HostConfig"])
			if err != nil {
				http.Error(w, "cannot prepare isolated job volumes", http.StatusInternalServerError)
				return
			}
			if len(host) > 0 {
				body["HostConfig"] = host
			}
			data, err := json.Marshal(body)
			if err != nil {
				http.Error(w, "invalid container configuration", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			r.ContentLength = int64(len(data))
		case r.Method == http.MethodGet && path == "/containers/json":
			query := r.URL.Query()
			filters := map[string]map[string]bool{}
			if raw := query.Get("filters"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &filters); err != nil || filters == nil {
					http.Error(w, "invalid container filters", http.StatusBadRequest)
					return
				}
			}
			if filters["label"] == nil {
				filters["label"] = map[string]bool{}
			}
			filters["label"][projectLabel+"="+id] = true
			data, _ := json.Marshal(filters)
			query.Set("filters", string(data))
			r.URL.RawQuery = query.Encode()
		case r.Method == http.MethodGet && path == "/volumes":
			// act identifies volumes only by shared names, so it cannot safely delete them.
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"Volumes":[],"Warnings":[]}`)
			return
		case r.Method == http.MethodDelete && (strings.HasPrefix(path, "/volumes/") || strings.HasPrefix(path, "/images/")):
			http.Error(w, "Docker resource removal is not allowed", http.StatusForbidden)
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(path, "/containers/"):
			query := r.URL.Query()
			query.Set("v", "0")
			r.URL.RawQuery = query.Encode()
		}
		proxy.ServeHTTP(w, r)
	})
}

func startDockerProxy(ctx context.Context, cli client.APIClient, id string) (string, func(), error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("start Docker proxy: %w", err)
	}
	server := &http.Server{Handler: dockerProxy(cli, id), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return "tcp://" + listener.Addr().String(), func() { _ = server.Close() }, nil
}
