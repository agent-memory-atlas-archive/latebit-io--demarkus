package answerbench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/latebit-io/demarkus/client/fetch"
	"github.com/latebit-io/demarkus/protocol"
	"github.com/latebit-io/demarkus/protocol/store"
)

func verifyEndpointFree(port int) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		return fmt.Errorf("fixture endpoint 127.0.0.1:%d occupied: %w", port, err)
	}
	return conn.Close()
}

func createRunCapability(work string) (raw, policyFile, capabilityFile string, err error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", "", "", fmt.Errorf("generate fixture capability: %w", err)
	}
	raw = hex.EncodeToString(secret)
	policyFile = filepath.Join(work, "tokens.toml")
	body := fmt.Sprintf("[tokens.answerbench]\nhash = %q\npaths = [\"/**\"]\noperations = [\"read\"]\n", protocol.HashToken(raw))
	if err := os.WriteFile(policyFile, []byte(body), 0o600); err != nil {
		return "", "", "", fmt.Errorf("write fixture capability policy: %w", err)
	}
	capabilityFile = filepath.Join(work, "reader-capability")
	if err := os.WriteFile(capabilityFile, []byte(raw+"\n"), 0o600); err != nil {
		return "", "", "", fmt.Errorf("write reader capability: %w", err)
	}
	return raw, policyFile, capabilityFile, nil
}

func verifyServerIndex(ctx context.Context, host, token, root, logPath string, server *managedProcess, f *Fixture) error {
	index, err := f.Section(Evidence{Path: "/index.md", Version: f.Latest("/index.md")})
	if err != nil {
		return err
	}
	if !index.Found {
		return errors.New("fixture /index.md revision missing; cannot verify server readiness")
	}
	client := fetch.NewClient(fetch.Options{Insecure: true})
	defer client.Close()
	version := f.Latest("/index.md")
	return waitForOwnedIndex(ctx, index.Text, version, store.ContentHash([]byte(index.Text)), root, host, server, func() ([]byte, error) {
		return os.ReadFile(logPath)
	}, func(probe context.Context) (fetch.Result, error) {
		return client.FetchContext(probe, host, "/index.md", "")
	}, func(probe context.Context) (fetch.Result, error) {
		return client.FetchContext(probe, host, "/index.md", token)
	})
}

func seed(root string, fixture *Fixture) error {
	if err := os.Mkdir(root, 0o700); err != nil {
		return err
	}
	s := store.New(root)
	for _, doc := range fixture.Documents {
		for version, body := range doc.Versions {
			if _, err := s.WriteVersion(doc.Path, version, []byte(body), doc.Metadata); err != nil {
				return fmt.Errorf("seed %s version %d: %w", doc.Path, version+1, err)
			}
		}
	}
	fixed := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			return os.Chtimes(path, fixed, fixed)
		}
		return nil
	})
}

func waitForIndex(ctx context.Context, index string, fetchIndex func(context.Context) (fetch.Result, error)) error {
	var last error
	for {
		probe, stop := context.WithTimeout(ctx, 300*time.Millisecond)
		res, err := fetchIndex(probe)
		stop()
		if err == nil && res.Response.Status == "ok" && res.Response.Body == index {
			return nil
		}
		last = err
		if err == nil {
			last = fmt.Errorf("unexpected index response: status=%q body-match=%t", res.Response.Status, res.Response.Body == index)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fixture server did not start: %w (last probe: %v)", ctx.Err(), last)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func waitForOwnedIndex(ctx context.Context, index string, version int, contentHash, root, host string, server *managedProcess, readLog func() ([]byte, error), fetchUnauthenticated, fetchIndex func(context.Context) (fetch.Result, error)) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	started := false
	var last error
	for {
		select {
		case <-server.Done():
			return fmt.Errorf("fixture server exited before readiness: %v", server.Err())
		default:
		}
		logs, err := readLog()
		if err != nil {
			last = fmt.Errorf("read fixture server log: %w", err)
		} else if ok, parseErr := hasOwnedStartup(logs, root, host); parseErr != nil {
			last = parseErr
		} else {
			started = ok
		}
		if started {
			probe, stop := context.WithTimeout(ctx, 300*time.Millisecond)
			unauthenticated, authErr := fetchUnauthenticated(probe)
			stop()
			switch {
			case authErr != nil:
				last = fmt.Errorf("unauthenticated fixture probe: %w", authErr)
			case unauthenticated.Response.Status != protocol.StatusUnauthorized:
				last = fmt.Errorf("fixture endpoint did not enforce run capability: status=%q", unauthenticated.Response.Status)
			default:
				probe, stop = context.WithTimeout(ctx, 300*time.Millisecond)
				res, err := fetchIndex(probe)
				stop()
				if err == nil && res.Response.Status == "ok" && res.Response.Body == index && res.Response.Metadata["version"] == strconv.Itoa(version) && res.Response.Metadata["content-hash"] == contentHash {
					select {
					case <-server.Done():
						return fmt.Errorf("fixture server exited during readiness: %v", server.Err())
					default:
						return nil
					}
				}
				last = err
				if err == nil {
					last = fmt.Errorf("unexpected authenticated index response: status=%q body-match=%t version=%q content-hash-match=%t", res.Response.Status, res.Response.Body == index, res.Response.Metadata["version"], res.Response.Metadata["content-hash"] == contentHash)
				}
			}
		}
		select {
		case <-server.Done():
			return fmt.Errorf("fixture server exited before readiness: %v", server.Err())
		case <-ctx.Done():
			return fmt.Errorf("owned fixture server did not become ready: %w (last check: %v)", ctx.Err(), last)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func hasOwnedStartup(raw []byte, root, host string) (bool, error) {
	_, expectedPort, err := net.SplitHostPort(host)
	if err != nil {
		return false, fmt.Errorf("invalid expected fixture address %q", host)
	}
	var parseErr error
	for line := range strings.SplitSeq(string(raw), "\n") {
		if line == "" {
			continue
		}
		var record struct {
			Message string `json:"msg"`
			Root    string `json:"root"`
			Addr    string `json:"addr"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			if parseErr == nil {
				parseErr = fmt.Errorf("parse fixture startup log: %w", err)
			}
			continue
		}
		if record.Message != "server started" {
			continue
		}
		if filepath.Clean(record.Root) != filepath.Clean(root) {
			return false, fmt.Errorf("fixture server started with root %q, want %q", record.Root, root)
		}
		_, port, err := net.SplitHostPort(record.Addr)
		if err != nil {
			return false, fmt.Errorf("fixture server reported invalid address %q", record.Addr)
		}
		if _, err := strconv.Atoi(port); err != nil {
			return false, fmt.Errorf("fixture server reported invalid port %q", port)
		}
		if port != expectedPort {
			return false, fmt.Errorf("fixture server started on port %s, want %s", port, expectedPort)
		}
		return true, nil
	}
	return false, parseErr
}
