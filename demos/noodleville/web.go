package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
)

//go:embed assets/*
var webAssets embed.FS

type WebState struct {
	Snapshot     WorldSnapshot  `json:"snapshot"`
	Feed         []Event        `json:"feed"`
	MemoryCounts map[string]int `json:"memory_counts"`
}

type ReportBroker struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	latest  []byte
}

func NewReportBroker() *ReportBroker {
	return &ReportBroker{clients: make(map[chan []byte]struct{})}
}

func (b *ReportBroker) Publish(report *TickReport) {
	data, err := json.Marshal(report)
	if err != nil {
		return
	}
	b.mu.Lock()
	b.latest = data
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
		}
	}
	b.mu.Unlock()
}

func (b *ReportBroker) ServeSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan []byte, 8)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	latest := append([]byte(nil), b.latest...)
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.clients, ch)
		b.mu.Unlock()
		close(ch)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if len(latest) > 0 {
		fmt.Fprintf(w, "event: tick\ndata: %s\n\n", latest)
		flusher.Flush()
	}
	for {
		select {
		case data := <-ch:
			fmt.Fprintf(w, "event: tick\ndata: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func NewWebHandler(ctx context.Context, town *Town, broker *ReportBroker) (http.Handler, error) {
	staticFS, err := fs.Sub(webAssets, "assets")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(WebState{
			Snapshot:     town.Snapshot(),
			Feed:         town.world.Feed(),
			MemoryCounts: town.MemoryCounts(ctx),
		})
	})
	mux.HandleFunc("/api/inspect", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		inspector, err := town.InspectVillager(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(inspector)
	})
	mux.HandleFunc("/api/events", broker.ServeSSE)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, err := fs.ReadFile(staticFS, "index.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
	})
	return mux, nil
}
