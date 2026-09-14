package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/latebit-io/demarkus/tools/internal/answerproxy"
	"github.com/latebit-io/demarkus/tools/internal/mcpstdio"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	binary := flag.String("mcp-bin", "", "production MCP binary")
	host := flag.String("host", "", "fixture host:port")
	dial := flag.String("dial-address", "", "network route for the frozen origin")
	tokenFile := flag.String("token-file", "", "private run capability file")
	scope := flag.String("scope", "", "allowed frozen path scope")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("takes flags only; unexpected argument %q", flag.Arg(0))
	}
	if *binary == "" || *host == "" || *tokenFile == "" {
		return fmt.Errorf("requires -mcp-bin, -host, and -token-file")
	}
	token, err := readCapability(*tokenFile)
	if err != nil {
		return err
	}
	args := []string{"-host", "mark://" + *host, "-insecure", "-no-cache"}
	if *dial != "" {
		args = append(args, "-dial-address", *dial)
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "DEMARKUS_AUTH=") {
			env = append(env, entry)
		}
	}
	env = append(env, "DEMARKUS_AUTH="+token)
	return answerproxy.Serve(ctx, mcpstdio.Config{Command: *binary, Args: args, Env: env}, *host, *scope)
}

func readCapability(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read run capability: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", fmt.Errorf("run capability is empty")
	}
	return token, nil
}
