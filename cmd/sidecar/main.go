// Package main is the ember-claw sidecar entry point.
//
// The sidecar imports PicoClaw as a Go library and exposes it via gRPC on
// port 50051. It also starts an HTTP health server on port 8080 for K8s
// liveness (/health) and readiness (/ready) probes (K8S-04).
//
// When channels are configured (e.g., Telegram), the sidecar also starts
// PicoClaw's channel manager (gateway mode) alongside the gRPC server.
//
// Config is resolved via the standard PicoClaw priority chain:
//
//	PICOCLAW_CONFIG env var (full path to config.json)
//	$PICOCLAW_HOME/config.json
//	~/.picoclaw/config.json
//
// In a container, set PICOCLAW_HOME=/data/.picoclaw (the PVC mount path).
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	_ "github.com/sipeed/picoclaw/pkg/channels/telegram"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/health"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/providers"
	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	emberclaw "github.com/tuomas-lb/ember-claw/gen/emberclaw/v1"
	"github.com/tuomas-lb/ember-claw/internal/server"
	lineartools "github.com/tuomas-lb/ember-claw/internal/tools/linear"
	slacktools "github.com/tuomas-lb/ember-claw/internal/tools/slack"
)

func main() {
	// --- Config loading (Pattern 3) ---
	configPath := getConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", configPath).Msg("failed to load picoclaw config")
	}
	log.Info().Str("path", configPath).Msg("config loaded")

	// --- Provider creation (Pattern 1) ---
	provider, modelID, err := providers.CreateProvider(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create LLM provider")
	}
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}
	log.Info().Str("model", cfg.Agents.Defaults.ModelName).Msg("provider initialized")

	// --- AgentLoop creation (Pattern 1) ---
	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)
	defer agentLoop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Initialize Backlog.md workspace if not already done ---
	initBacklog(cfg.WorkspacePath())

	// --- Register ember-claw tools (conditionally, based on env vars) ---
	registerTools(agentLoop)

	// MUST start this goroutine (Pitfall 3: background tasks require Run)
	go agentLoop.Run(ctx)
	log.Info().Msg("agent loop started")

	// --- Channel manager (gateway mode) ---
	// Start channels (Telegram, etc.) if any are configured.
	var channelMgr *channels.Manager
	if hasChannelsEnabled(cfg) {
		channelMgr, err = startChannels(ctx, cfg, agentLoop, msgBus)
		if err != nil {
			log.Error().Err(err).Msg("failed to start channels (continuing without)")
		}
	}

	// --- Health server (Pattern 4) ---
	// PicoClaw's health.Server exposes /health and /ready for K8s probes.
	healthSrv := health.NewServer("0.0.0.0", 8080)

	// If channels need webhooks, set up HTTP server on the health server.
	if channelMgr != nil {
		addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
		if addr == ":0" || addr == ":" {
			addr = "0.0.0.0:8080"
		}
		channelMgr.SetupHTTPServer(addr, healthSrv)
	}

	healthSrv.SetReady(true)
	go func() {
		if err := healthSrv.Start(); err != nil {
			log.Error().Err(err).Msg("health server stopped")
		}
	}()
	log.Info().Int("port", 8080).Msg("health server started")

	// --- gRPC server (Pattern 6) ---
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen on :50051")
	}

	grpcSrv := grpc.NewServer()

	// Wire the PicoClaw service
	svc := server.New(agentLoop)
	svc.SetModel(cfg.Agents.Defaults.ModelName)
	svc.SetProvider(modelID)
	svc.SetReady(true)
	emberclaw.RegisterPicoClawServiceServer(grpcSrv, svc)

	// Wire the standard gRPC health service (K8S-04: K8s 1.24+ gRPC probes)
	grpcHealthSrv := grpchealth.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, grpcHealthSrv)
	grpcHealthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			log.Error().Err(err).Msg("gRPC server stopped")
		}
	}()
	log.Info().Int("port", 50051).Msg("gRPC server started")

	// --- Graceful shutdown ---
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	log.Info().Msg("shutting down")
	if channelMgr != nil {
		channelMgr.StopAll(ctx)
	}
	grpcSrv.GracefulStop()
	agentLoop.Stop()
	log.Info().Msg("shutdown complete")
}

// hasChannelsEnabled checks if any messaging channel is enabled in the config.
func hasChannelsEnabled(cfg *config.Config) bool {
	return cfg.Channels.Telegram.Enabled ||
		cfg.Channels.Discord.Enabled ||
		cfg.Channels.Slack.Enabled ||
		cfg.Channels.WhatsApp.Enabled
}

// startChannels initializes and starts the PicoClaw channel manager.
func startChannels(ctx context.Context, cfg *config.Config, agentLoop *agent.AgentLoop, msgBus *bus.MessageBus) (*channels.Manager, error) {
	mediaStore := media.NewFileMediaStoreWithCleanup(media.MediaCleanerConfig{
		Enabled: false, // Keep it simple for container mode
	})

	mgr, err := channels.NewManager(cfg, msgBus, mediaStore)
	if err != nil {
		return nil, fmt.Errorf("create channel manager: %w", err)
	}

	agentLoop.SetChannelManager(mgr)
	agentLoop.SetMediaStore(mediaStore)

	if err := mgr.StartAll(ctx); err != nil {
		return nil, fmt.Errorf("start channels: %w", err)
	}

	enabled := mgr.GetEnabledChannels()
	log.Info().Strs("channels", enabled).Msg("channels started")
	return mgr, nil
}

// registerTools registers ember-claw tools (Linear, Slack) to the agent loop.
// Tools are only registered when their respective env vars are set.
func registerTools(agentLoop *agent.AgentLoop) {
	if apiKey := os.Getenv("LINEAR_API_KEY"); apiKey != "" {
		teamID := os.Getenv("LINEAR_TEAM_ID")
		client := lineartools.NewClient(apiKey)
		agentLoop.RegisterTool(lineartools.NewCreateIssueTool(client, teamID))
		agentLoop.RegisterTool(lineartools.NewSearchIssuesTool(client, teamID))
		agentLoop.RegisterTool(lineartools.NewGetIssueTool(client))
		agentLoop.RegisterTool(lineartools.NewUpdateIssueTool(client, teamID))
		log.Info().Str("teamID", teamID).Msg("linear tools registered")
	}

	if botToken := os.Getenv("SLACK_BOT_TOKEN"); botToken != "" {
		client := slacktools.NewClient(botToken)
		agentLoop.RegisterTool(slacktools.NewSendMessageTool(client))
		agentLoop.RegisterTool(slacktools.NewListChannelsTool(client))
		log.Info().Msg("slack tools registered")
	}
}

// getConfigPath resolves the PicoClaw config file path using the standard
// priority chain (Pitfall 1: container must set PICOCLAW_HOME).
//
//  1. PICOCLAW_CONFIG env var (full path)
//  2. $PICOCLAW_HOME/config.json
//  3. ~/.picoclaw/config.json
func getConfigPath() string {
	if p := os.Getenv("PICOCLAW_CONFIG"); p != "" {
		return p
	}
	home := os.Getenv("PICOCLAW_HOME")
	if home != "" {
		return home + "/config.json"
	}
	h, err := os.UserHomeDir()
	if err != nil {
		log.Warn().Err(err).Msg("could not determine home dir; using /root/.picoclaw")
		h = "/root"
	}
	return h + "/.picoclaw/config.json"
}

// initBacklog initializes Backlog.md in the workspace if not already done.
// Runs `backlog init` non-interactively to create the backlog/ directory structure.
func initBacklog(workspace string) {
	backlogDir := filepath.Join(workspace, "backlog")
	if _, err := os.Stat(backlogDir); err == nil {
		log.Info().Str("dir", backlogDir).Msg("backlog already initialized")
		return
	}

	// Ensure workspace exists.
	if err := os.MkdirAll(workspace, 0755); err != nil {
		log.Warn().Err(err).Msg("failed to create workspace for backlog")
		return
	}

	// Also init git if not present (backlog.md requires a git repo).
	gitDir := filepath.Join(workspace, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		gitCmd := exec.Command("git", "init")
		gitCmd.Dir = workspace
		if out, err := gitCmd.CombinedOutput(); err != nil {
			log.Warn().Err(err).Str("output", string(out)).Msg("git init failed")
			return
		}
		// Set git user for commits inside workspace.
		for _, args := range [][]string{
			{"config", "user.email", "picoclaw@ember-claw.local"},
			{"config", "user.name", "PicoClaw"},
		} {
			c := exec.Command("git", args...)
			c.Dir = workspace
			_ = c.Run()
		}
		log.Info().Msg("git repo initialized in workspace")
	}

	cmd := exec.Command("backlog", "init", "PicoClaw Workspace", "--backlog-dir", "backlog")
	cmd.Dir = workspace
	cmd.Stdin = nil // non-interactive
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn().Err(err).Str("output", string(out)).Msg("backlog init failed (will retry on next restart)")
		return
	}
	log.Info().Str("dir", backlogDir).Msg("backlog initialized")
}
