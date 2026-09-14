package answerbench

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/latebit-io/demarkus/client/fetch"
	"github.com/latebit-io/demarkus/protocol"
	"github.com/latebit-io/demarkus/protocol/store"
)

func TestReadinessReportsFinalProbeReason(t *testing.T) {
	for _, tc := range []struct {
		name, status, body, want string
		failure                  error
	}{
		{"wrong-body", "ok", "another server", "body-match=false", nil},
		{"wrong-status", "not-found", "", `status="not-found"`, nil},
		{"transport", "", "", "connection refused", errors.New("connection refused")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err := waitForIndex(ctx, "# Expected", func(context.Context) (fetch.Result, error) {
				cancel()
				return fetch.Result{Response: protocol.Response{Status: tc.status, Body: tc.body}}, tc.failure
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("probe error=%v", err)
			}
		})
	}
	if err := waitForIndex(t.Context(), "# Expected", func(context.Context) (fetch.Result, error) {
		return fetch.Result{Response: protocol.Response{Status: "ok", Body: "# Expected"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointPreflightRejectsOccupiedPort(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	}()
	port := listener.LocalAddr().(*net.UDPAddr).Port
	if err := verifyEndpointFree(port); err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("occupied endpoint error=%v", err)
	}
}

func TestRunCapabilityIsHashedAndUnique(t *testing.T) {
	first, firstPath, firstRawPath, err := createRunCapability(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, _, _, err := createRunCapability(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || strings.Contains(string(raw), first) || !strings.Contains(string(raw), "sha256-") {
		t.Fatalf("capability was reused or stored raw: %s", raw)
	}
	readerRaw, err := os.ReadFile(firstRawPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(readerRaw)) != first {
		t.Fatal("reader capability file does not contain generated capability")
	}
}

func TestOwnedReadinessRequiresChildStartupAndLiveness(t *testing.T) {
	root := filepath.Join(t.TempDir(), "corpus")
	startup := []byte(`{"msg":"server started","addr":"[::]:16319","root":` + strconv.Quote(root) + `}` + "\n")
	contentHash := store.ContentHash([]byte("# Expected"))
	result := fetch.Result{Response: protocol.Response{Status: "ok", Body: "# Expected", Metadata: map[string]string{"version": "1", "content-hash": contentHash}}}
	unauthorized := fetch.Result{Response: protocol.Response{Status: protocol.StatusUnauthorized}}
	for _, tc := range []struct {
		name string
		log  []byte
		exit bool
		ok   bool
	}{
		{name: "owned", log: startup, ok: true},
		{name: "owned-after-malformed", log: append([]byte("not-json\n"), startup...), ok: true},
		{name: "matching-orphan", log: []byte(`{"msg":"lookup catalog built"}`)},
		{name: "owned-child-exited", log: startup, exit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			process := &managedProcess{done: make(chan struct{})}
			if tc.exit {
				close(process.done)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err := waitForOwnedIndex(ctx, "# Expected", 1, contentHash, root, "127.0.0.1:16319", process, func() ([]byte, error) {
				if !tc.ok && !tc.exit {
					cancel()
				}
				return tc.log, nil
			}, func(context.Context) (fetch.Result, error) {
				return unauthorized, nil
			}, func(context.Context) (fetch.Result, error) {
				return result, nil
			})
			if (err == nil) != tc.ok {
				t.Fatalf("readiness error=%v, want success=%t", err, tc.ok)
			}
		})
	}
}

func TestOwnedStartupReturnsMalformedLogErrorWithoutValidRecord(t *testing.T) {
	if ok, err := hasOwnedStartup([]byte("not-json\n"), t.TempDir(), "127.0.0.1:16319"); ok || err == nil || !strings.Contains(err.Error(), "parse fixture startup log") {
		t.Fatalf("owned startup=%t error=%v", ok, err)
	}
}

func TestOwnedReadinessRejectsPublicMatchingFixture(t *testing.T) {
	root := filepath.Join(t.TempDir(), "corpus")
	startup := []byte(`{"msg":"server started","addr":"[::]:16319","root":` + strconv.Quote(root) + `}` + "\n")
	contentHash := store.ContentHash([]byte("# Expected"))
	result := fetch.Result{Response: protocol.Response{Status: "ok", Body: "# Expected", Metadata: map[string]string{"version": "1", "content-hash": contentHash}}}
	process := &managedProcess{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	err := waitForOwnedIndex(ctx, "# Expected", 1, contentHash, root, "127.0.0.1:16319", process, func() ([]byte, error) {
		return startup, nil
	}, func(context.Context) (fetch.Result, error) {
		cancel()
		return result, nil
	}, func(context.Context) (fetch.Result, error) {
		return result, nil
	})
	if err == nil || !strings.Contains(err.Error(), "run capability") {
		t.Fatalf("public matching fixture error=%v", err)
	}
}
