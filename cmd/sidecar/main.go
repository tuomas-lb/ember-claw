// Package main is the ember-claw sidecar entry point.
//
// The sidecar imports PicoClaw as a Go library and exposes it via gRPC on
// port 50051. It also starts an HTTP health server on port 8080 for K8s
// liveness (/health) and readiness (/ready) probes (K8S-04).
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
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/health"
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

	// --- Register ember-claw tools (conditionally, based on env vars) ---
	registerTools(agentLoop)

	// MUST start this goroutine (Pitfall 3: background tasks require Run)
	go agentLoop.Run(ctx)
	log.Info().Msg("agent loop started")

	// --- Health server (Pattern 4) ---
	// PicoClaw's health.Server exposes /health and /ready for K8s probes.
	healthSrv := health.NewServer("0.0.0.0", 8080)
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
	grpcSrv.GracefulStop()
	agentLoop.Stop()
	log.Info().Msg("shutdown complete")
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
//	1. PICOCLAW_CONFIG env var (full path)
//	2. $PICOCLAW_HOME/config.json
//	3. ~/.picoclaw/config.json
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
