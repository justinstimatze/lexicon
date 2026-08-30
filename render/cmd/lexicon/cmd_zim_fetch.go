package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

func cmdZimFetch(renderDir string, args []string) {
	fs := flag.NewFlagSet("zim-fetch", flag.ExitOnError)
	port := fs.Int("port", 18080, "gozimhttpd port")
	slug := fs.String("slug", "wikipedia-en-all-nopic-2025-12", "ZIM slug (matches the filename minus .zim, with _ -> -)")
	raw := fs.Bool("raw", false, "emit raw HTML instead of pandoc-cleaned text")
	timeout := fs.Duration("timeout", 30*time.Second, "request timeout")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() < 1 {
		fatal("zim-fetch: <article-title> required (e.g. lexicon zim-fetch \"Hanlon's razor\")")
	}
	// Accept either "Hanlon's_razor" or "Hanlon's razor" — collapse spaces to _.
	title := strings.ReplaceAll(strings.Join(fs.Args(), " "), " ", "_")
	encoded := url.PathEscape(title)

	target := fmt.Sprintf("http://localhost:%d/%s/C/%s", *port, *slug, encoded)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		fatal("build request: %v", err)
	}
	client := &http.Client{Timeout: *timeout}
	resp, err := client.Do(req)
	if err != nil {
		fatal("connect to gozimhttpd at localhost:%d failed: %v\n"+
			"start it with: gozimhttpd -path <wikipedia.zim> -port %d", *port, err, *port)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		fatal("HTTP %d for %s (try a different title or check the ZIM slug)", resp.StatusCode, target)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fatal("read body: %v", err)
	}

	if *raw {
		if _, err := os.Stdout.Write(body); err != nil {
			fatal("write stdout: %v", err)
		}
		return
	}

	cmd := exec.Command("pandoc", "-f", "html", "-t", "plain", "--wrap=none")
	cmd.Stdin = strings.NewReader(string(body))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("pandoc html->plain failed: %v (try --raw)", err)
	}
}
