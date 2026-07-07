package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	dashboard "github.com/tuomas-lb/ember-claw/dashboard"
	"github.com/tuomas-lb/ember-claw/dashboard/internal/api"
	grpcclient "github.com/tuomas-lb/ember-claw/dashboard/internal/grpc"
	"github.com/tuomas-lb/ember-claw/dashboard/internal/k8s"
	"github.com/tuomas-lb/ember-claw/dashboard/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	namespace := envOr("NAMESPACE", "picoclaw")
	addr := envOr("ADDR", ":8090")

	k8sClient, err := k8s.New(namespace)
	if err != nil {
		log.Fatalf("create k8s client: %v", err)
	}

	grpcClient := grpcclient.New(namespace)

	// Optional chat persistence: enabled when DATABASE_URL is set.
	var chatStore *store.Store
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		chatStore, err = store.New(context.Background(), dbURL)
		if err != nil {
			log.Fatalf("connect chat store: %v", err)
		}
		defer chatStore.Close()
		log.Printf("chat persistence enabled (postgres)")
	} else {
		log.Printf("chat persistence disabled (DATABASE_URL not set)")
	}

	h := api.NewHandler(k8sClient, grpcClient, chatStore)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// API routes. Responses are dynamic (chat history, instance state) and must
	// never be served from a browser/proxy cache — otherwise a refresh can show
	// stale history.
	r.Route("/api", func(r chi.Router) {
		r.Use(noStore)
		r.Get("/instances", h.ListInstances)
		r.Post("/instances", h.DeployInstance)
		r.Get("/instances/{name}", h.GetInstance)
		r.Delete("/instances/{name}", h.DeleteInstance)
		r.Post("/instances/{name}/restart", h.RestartInstance)
		r.Get("/instances/{name}/config", h.GetConfig)
		r.Put("/instances/{name}/config", h.PushConfig)
		r.Post("/instances/{name}/secret", h.SetSecret)
		r.Get("/instances/{name}/status", h.GetInstanceStatus)
		r.Get("/instances/{name}/logs", h.HandleLogs)
		r.Get("/instances/{name}/chat", h.HandleChat)
		r.Get("/instances/{name}/messages", h.ListMessages)
		r.Get("/instances/{name}/sessions", h.ListSessions)
		r.Get("/providers", h.ListProviders)
		r.Get("/providers/{provider}/models", h.ListModels)
		r.Get("/fleet", h.GetFleetMD)
		r.Put("/fleet", h.PutFleetMD)
		r.Get("/telephony/routing", h.GetCallRouting)
		r.Put("/telephony/routing", h.PutCallRouting)
		r.Post("/telephony/restart", h.RestartVoiceBridge)
	})

	// SPA — serve embedded frontend; fall back to index.html for unknown paths.
	webDist, err := fs.Sub(dashboard.WebFS, "web/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	indexHTML, err := fs.ReadFile(dashboard.WebFS, "web/dist/index.html")
	if err != nil {
		log.Fatalf("read index.html: %v", err)
	}
	r.Handle("/*", spaHandler(http.FS(webDist), indexHTML))

	log.Printf("Dashboard listening on %s (namespace: %s)", addr, namespace)
	log.Fatal(http.ListenAndServe(addr, r))
}

// noStore marks responses as non-cacheable (dynamic API data).
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// spaHandler serves the embedded frontend. The index.html shell is served
// no-cache so the browser always picks up the current (content-hashed) JS/CSS
// bundle — otherwise a cached shell keeps loading a stale frontend. The hashed
// assets themselves are immutable and cached long.
func spaHandler(fsys http.FileSystem, index []byte) http.Handler {
	fileServer := http.FileServer(fsys)
	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.Write(index)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "/index.html" {
			serveIndex(w)
			return
		}
		if f, err := fsys.Open(path); err == nil {
			f.Close()
			// Vite emits content-hashed filenames under /assets — safe to cache forever.
			if strings.HasPrefix(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		// Unknown path — SPA client-side route; serve the shell.
		serveIndex(w)
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
