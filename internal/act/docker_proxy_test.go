package act

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
	actcontainer "github.com/nektos/act/pkg/container"
)

type proxyClient struct {
	client.APIClient
	address string
}

func (c proxyClient) Dialer() func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) { return (&net.Dialer{}).DialContext(ctx, "tcp", c.address) }
}

func TestDockerProxyScopesAndForwards(t *testing.T) {
	var creates, lists, deletes int
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.45/containers/create":
			creates++
			var body struct {
				Labels map[string]string `json:"Labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Labels[projectLabel] != "project" || body.Labels["other"] != "value" {
				t.Errorf("create labels: %+v %v", body, err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"Id":"created"}`)
		case "/v1.45/containers/json":
			lists++
			var filters map[string]map[string]bool
			if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil || !filters["label"][projectLabel+"=project"] || !filters["status"]["running"] {
				t.Errorf("filters: %+v %v", filters, err)
			}
			_, _ = io.WriteString(w, `[]`)
		case "/v1.45/containers/created":
			deletes++
			if r.URL.Query().Get("v") != "0" {
				t.Error("anonymous volumes would be removed")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected upstream request %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer daemon.Close()
	proxy := httptest.NewServer(dockerProxy(proxyClient{address: strings.TrimPrefix(daemon.URL, "http://")}, "project"))
	defer proxy.Close()
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/v1.45/containers/create?name=job", `{"Labels":{"other":"value","io.acton.project":"forged"}}`, 201},
		{"GET", "/v1.45/containers/json?filters=%7B%22status%22%3A%7B%22running%22%3Atrue%7D%7D", "", 200},
		{"DELETE", "/v1.45/containers/created?v=1", "", 204},
		{"POST", "/containers/create", `{`, 400},
		{"GET", "/containers/json?filters=bad", "", 400},
		{"GET", "/volumes", "", 200},
		{"DELETE", "/volumes/foreign", "", 403},
		{"DELETE", "/images/foreign", "", 403},
	} {
		req, err := http.NewRequestWithContext(t.Context(), tc.method, proxy.URL+tc.path, bytes.NewBufferString(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, resp.StatusCode, tc.status)
		}
	}
	if creates != 1 || lists != 1 || deletes != 1 {
		t.Fatalf("forwarded create/list/delete: %d/%d/%d", creates, lists, deletes)
	}
	daemon.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, proxy.URL+"/_ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("upstream failure: %d", resp.StatusCode)
	}
}

func TestDockerProxyUpgrade(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = rw.Flush()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(rw, buf); err != nil {
			t.Error(err)
			return
		}
		_, _ = rw.Write(buf)
		_ = rw.Flush()
	}))
	defer daemon.Close()
	proxy := httptest.NewServer(dockerProxy(proxyClient{address: strings.TrimPrefix(daemon.URL, "http://")}, "project"))
	defer proxy.Close()
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", strings.TrimPrefix(proxy.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = io.WriteString(conn, "POST /v1.45/exec/id/start HTTP/1.1\r\nHost: docker\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Length: 0\r\n\r\n")
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade: %v %v", response, err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.WriteString(conn, "test")
	buf := make([]byte, 4)
	if _, err := io.ReadFull(reader, buf); err != nil || string(buf) != "test" {
		t.Fatalf("stream: %q %v", buf, err)
	}
}

func TestDockerProxyRealDaemon(t *testing.T) {
	if os.Getenv("ACTON_TEST_DOCKER") != "1" {
		t.Skip("set ACTON_TEST_DOCKER=1 for the local Docker fixture")
	}
	cli, err := actcontainer.GetDockerClient(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli.Close() }()
	id := fmt.Sprintf("acton-test-%d", time.Now().UnixNano())
	foreign := id + "-foreign"
	//nolint:gosec // G204: the image and arguments are fixed; foreign is a test-generated container name.
	cmd := exec.CommandContext(t.Context(), "docker", "create", "--name", foreign, "postgres:16-alpine")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create fixture: %s %v", output, err)
	}
	defer func() {
		//nolint:gosec // G204: foreign is a test-generated container name.
		_ = exec.CommandContext(context.WithoutCancel(t.Context()), "docker", "rm", "-f", foreign).Run()
	}()
	defer func() {
		if err := cleanupContainers(context.Background(), cli, id); err != nil {
			t.Error(err)
		}
	}()
	host, closeProxy, err := startDockerProxy(t.Context(), cli, id)
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	base := "http://" + strings.TrimPrefix(host, "tcp://")
	create := func(name string) int {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/v1.45/containers/create?name="+name, strings.NewReader(`{"Image":"postgres:16-alpine"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	if status := create(foreign); status != http.StatusConflict {
		t.Fatalf("foreign name conflict: %d", status)
	}
	if status := create(id + "-owned"); status != http.StatusCreated {
		t.Fatalf("owned creation: %d", status)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/v1.45/containers/json?all=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var items []struct {
		Names  []string          `json:"Names"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	found := false
	for _, item := range items {
		for _, name := range item.Names {
			if name == "/"+foreign {
				t.Fatal("foreign container visible")
			}
			if name == "/"+id+"-owned" && item.Labels[projectLabel] == id {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("owned container missing")
	}
	if err := cleanupContainers(t.Context(), cli, id); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G204: foreign is a test-generated container name.
	if err := exec.CommandContext(t.Context(), "docker", "inspect", foreign).Run(); err != nil {
		t.Fatalf("foreign container removed: %v", err)
	}
	//nolint:gosec // G204: id is a test-generated container name.
	if err := exec.CommandContext(t.Context(), "docker", "inspect", id+"-owned").Run(); err == nil {
		t.Fatal("owned container survived cleanup")
	}
}
