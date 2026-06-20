package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/deepnoodle-ai/dive/demos/colosseum/web"
)

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dir := fs.String("dir", "transcripts", "directory of match transcripts to serve")
	artifactsDir := fs.String("artifacts", "artifacts", "directory of artifacts to serve in the Artifacts tab (optional)")
	addr := fs.String("addr", ":8723", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	srv, err := web.NewServer(*dir, web.WithArtifactsDir(*artifactsDir))
	if err != nil {
		return err
	}
	fmt.Printf("🏛  The Colosseum replay viewer\n")
	fmt.Printf("    serving transcripts from %q\n", *dir)
	if info, err := os.Stat(*artifactsDir); err == nil && info.IsDir() {
		fmt.Printf("    serving artifacts from %q\n", *artifactsDir)
	}
	fmt.Printf("    open http://localhost%s\n", normalizeAddr(*addr))
	return http.ListenAndServe(*addr, srv.Handler())
}

// normalizeAddr makes a bare ":8723" presentable in the printed URL.
func normalizeAddr(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		return addr
	}
	return ":" + addr
}
