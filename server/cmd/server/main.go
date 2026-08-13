package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kms9/dars/internal/daemonws"
	"github.com/kms9/dars/internal/events"
	"github.com/kms9/dars/internal/lightweightapi"
	"github.com/kms9/dars/internal/lightweightmigrations"
	"github.com/kms9/dars/internal/logger"
	"github.com/kms9/dars/internal/mcpgateway"
	obsmetrics "github.com/kms9/dars/internal/metrics"
	"github.com/kms9/dars/internal/realtime"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/redis/go-redis/v9"
)

var (
	version = "dev"
	commit  = "unknown"
)

func newNamedRedisClient(base *redis.Options, suffix string) *redis.Client {
	opts := *base
	if envBool("REDIS_DISABLE_CLIENT_NAME", false) {
		opts.ClientName = ""
	} else {
		opts.ClientName = redisClientName(opts.ClientName, suffix)
	}
	return redis.NewClient(&opts)
}

func redisClientName(existing, suffix string) string {
	if suffix == "" {
		return existing
	}
	if existing != "" {
		return existing + ":" + suffix
	}
	return "dars-api:" + suffix
}

func closeRedisClient(label string, client *redis.Client) {
	if client == nil {
		return
	}
	if err := client.Close(); err != nil {
		slog.Warn("redis client close failed", "client", label, "error", err)
	}
}

func shardedRelayConfigFromEnv() realtime.ShardedStreamRelayConfig {
	cfg := realtime.DefaultShardedStreamRelayConfig()
	cfg.Shards = envPositiveInt("REALTIME_RELAY_SHARDS", cfg.Shards)
	cfg.StreamMaxLen = envPositiveInt64("REALTIME_RELAY_STREAM_MAXLEN", cfg.StreamMaxLen)
	cfg.ReadCount = envPositiveInt64("REALTIME_RELAY_XREAD_COUNT", cfg.ReadCount)
	cfg.ReadBlock = envDuration("REALTIME_RELAY_XREAD_BLOCK", cfg.ReadBlock)
	cfg.ReplayGrace = envDuration("REALTIME_RELAY_REPLAY_GRACE", cfg.ReplayGrace)
	return cfg
}

func envPositiveInt(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		slog.Warn("invalid env var, using default", "name", name, "value", raw, "default", def, "error", err)
		return def
	}
	return v
}

func envPositiveInt64(name string, def int64) int64 {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		slog.Warn("invalid env var, using default", "name", name, "value", raw, "default", def, "error", err)
		return def
	}
	return v
}

func envDuration(name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v <= 0 {
		slog.Warn("invalid env var, using default", "name", name, "value", raw, "default", def.String(), "error", err)
		return def
	}
	return v
}

func envNonNegativeDuration(name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v < 0 {
		slog.Warn("invalid env var, using default", "name", name, "value", raw, "default", def.String(), "error", err)
		return def
	}
	return v
}

func holdBeforeShutdown(sig os.Signal, signals <-chan os.Signal, duration time.Duration) {
	if duration <= 0 {
		return
	}
	slog.Info("termination signal received; holding before shutdown",
		"signal", sig.String(),
		"duration", duration.String(),
	)
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		slog.Info("shutdown hold complete", "duration", duration.String())
	case interruptSig := <-signals:
		slog.Info("shutdown hold interrupted by signal",
			"signal", interruptSig.String(),
			"configured_duration", duration.String(),
		)
	}
}

func envBool(name string, def bool) bool {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		slog.Warn("invalid env var, using default", "name", name, "value", raw, "default", def, "error", err)
		return def
	}
	return v
}

func main() {
	logger.Init()

	// Warn about missing configuration
	if os.Getenv("JWT_SECRET") == "" {
		slog.Warn("JWT_SECRET is not set — using insecure default. Set JWT_SECRET for production use.")
	}
	if os.Getenv("RESEND_API_KEY") == "" && strings.TrimSpace(os.Getenv("SMTP_HOST")) == "" {
		slog.Warn("no email backend configured (RESEND_API_KEY and SMTP_HOST both empty) — verification codes will be printed to the log instead of emailed.")
	}
	if os.Getenv("DARS_DEV_VERIFICATION_CODE") != "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") {
			slog.Warn("DARS_DEV_VERIFICATION_CODE is set but ignored because APP_ENV=production.")
		} else {
			slog.Warn("DARS_DEV_VERIFICATION_CODE is enabled. Use it only for local development or private test instances.")
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	shutdownHoldDuration := envNonNegativeDuration("DARS_SHUTDOWN_HOLD_DURATION", 0)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://dars:dars@localhost:5432/dars_lightweight?sslmode=disable"
	}

	// Connect to database
	ctx := context.Background()
	pool, err := newDBPool(ctx, dbURL)
	if err != nil {
		slog.Error("unable to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("unable to ping database", "error", err)
		os.Exit(1)
	}
	if err := lightweightmigrations.ValidateRuntimeDatabaseIdentity(ctx, pool, expectedLightweightDatabaseName()); err != nil {
		slog.Error("database_identity_mismatch", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to database")
	logPoolConfig(pool)

	bus := events.New()
	hub := realtime.NewHub()
	go hub.Run()
	daemonHub := daemonws.NewHub()
	var daemonWakeup lightweightapi.DaemonWakeupNotifier = daemonHub

	// MUL-1138: when REDIS_URL is set, route fanout through a Redis relay so
	// multiple API nodes can deliver each other's events. Without it the hub
	// is the sole broadcaster and the server stays single-node (legacy).
	// Runtime local-skill stores and realtime relay traffic use separate Redis
	// clients so blocking stream consumers cannot starve request-path Redis
	// operations.
	relayCtx, relayCancel := context.WithCancel(context.Background())
	var broadcaster realtime.Broadcaster = hub
	var storeRedis *redis.Client
	var relayWriteRedis *redis.Client
	var relayReadRedis *redis.Client
	var relay realtime.ManagedRelay
	defer func() {
		if relay != nil {
			relay.Stop()
		}
		relayCancel()
		if relay != nil {
			relay.Wait()
		}
		closeRedisClient("realtime-read", relayReadRedis)
		closeRedisClient("realtime-write", relayWriteRedis)
		closeRedisClient("store", storeRedis)
	}()
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			slog.Error("invalid REDIS_URL — falling back to in-memory hub", "error", err)
		} else {
			if envBool("REDIS_DISABLE_CLIENT_NAME", false) {
				slog.Info("redis: CLIENT SETNAME disabled (REDIS_DISABLE_CLIENT_NAME=true) for managed Redis compatibility")
			}
			storeRedis = newNamedRedisClient(opts, "store")
			relayWriteRedis = newNamedRedisClient(opts, "realtime-write")

			relayConfig := shardedRelayConfigFromEnv()
			relayReadRedis = newNamedRedisClient(opts, "realtime-read")
			sharded := realtime.NewShardedStreamRelay(hub, relayWriteRedis, relayReadRedis, relayConfig)
			sharded.SetDaemonRuntimeDeliverer(daemonHub)
			relay = sharded
			daemonWakeup = daemonws.NewRelayNotifier(daemonHub, sharded)
			relay.Start(relayCtx)
			broadcaster = realtime.NewDualWriteBroadcaster(hub, relay)
			slog.Info(
				"realtime: Redis relay enabled",
				"node_id", relay.NodeID(),
				"mode", "sharded",
				"shards", relayConfig.Shards,
				"stream_max_len", relayConfig.StreamMaxLen,
				"xread_count", relayConfig.ReadCount,
				"xread_block", relayConfig.ReadBlock.String(),
				"store_pool_size", opts.PoolSize,
				"realtime_write_pool_size", opts.PoolSize,
				"realtime_read_pool_size", opts.PoolSize,
			)
		}
	} else {
		slog.Info("realtime: REDIS_URL not set — using in-memory hub (single-node mode)")
	}
	registerLightweightListeners(bus, broadcaster)

	queries := lwdb.New(pool)
	hub.SetAuthorizer(newScopeAuthorizer(queries))

	metricsConfig := obsmetrics.ConfigFromEnv()
	var metricsServer *http.Server
	var httpMetrics *obsmetrics.HTTPMetrics
	var gatewayObserver mcpgateway.Observer
	if metricsConfig.Enabled() {
		metricsRegistry := obsmetrics.NewRuntimeRegistry(obsmetrics.RegistryOptions{
			Realtime: realtime.M,
			DaemonWS: daemonws.M,
			Version:  version,
			Commit:   commit,
		})
		httpMetrics = metricsRegistry.HTTP
		gatewayObserver = metricsRegistry.Gateway
		metricsServer = obsmetrics.NewServer(metricsConfig.Addr, metricsRegistry.Gatherer)
		if !obsmetrics.IsLoopbackAddr(metricsConfig.Addr) {
			slog.Warn(
				"metrics listener is not loopback-only; restrict access with private networking, allowlists, or proxy auth",
				"addr", metricsConfig.Addr,
			)
		}
	}
	// Construct the BatchedHeartbeatScheduler before the router so it can
	// be injected into the Handler. The Run goroutine starts below
	// alongside the sweeper, and Stop is called explicitly during graceful
	// shutdown so any pending bumps are flushed before we exit.
	heartbeatScheduler := lightweightapi.NewBatchedHeartbeatScheduler(queries, lightweightapi.DefaultHeartbeatBatchInterval)

	composition, err := NewLightweightComposition(pool, hub, bus, storeRedis, LightweightCompositionOptions{
		HTTPMetrics:        httpMetrics,
		GatewayObserver:    gatewayObserver,
		DaemonHub:          daemonHub,
		DaemonWakeup:       daemonWakeup,
		HeartbeatScheduler: heartbeatScheduler,
	})
	if err != nil {
		slog.Error("server composition failed", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: composition.Router,
	}

	// The Lightweight worker allowlist contains exactly the runtime/task
	// sweeper and batched heartbeat scheduler here. The in-memory hub, optional
	// Redis relay and optional metrics listener are started in their dedicated
	// lifecycle blocks above/below.
	sweepCtx, sweepCancel := context.WithCancel(context.Background())
	go runLightweightTaskSweeper(sweepCtx, composition.Core)
	go heartbeatScheduler.Run(sweepCtx)

	if metricsServer != nil {
		go func() {
			slog.Info("metrics server starting", "addr", metricsConfig.Addr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("metrics server disabled after startup error", "error", err)
			}
		}()
	}

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	holdBeforeShutdown(sig, quit, shutdownHoldDuration)
	// Restore the default behavior so another signal during graceful shutdown
	// can still terminate the process instead of being left unread in quit.
	signal.Stop(quit)

	slog.Info("shutting down server")

	// Order matters: drain in-flight HTTP first so any heartbeat handlers
	// finish calling Schedule() before we stop the scheduler. Otherwise a
	// late heartbeat could enqueue a pending ID after Run has already
	// drained and exited, and Stop() would not flush it.
	apiShutdownCtx, apiShutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := srv.Shutdown(apiShutdownCtx); err != nil {
		apiShutdownCancel()
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	apiShutdownCancel()
	gatewayShutdownCtx, gatewayShutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := composition.Gateway.Close(gatewayShutdownCtx); err != nil {
		slog.Error("MCP Gateway forced to shutdown", "error", err)
	}
	gatewayShutdownCancel()

	// HTTP is fully drained — safe to stop the sweeper and flush the
	// final batch of queued heartbeat bumps.
	sweepCancel()
	heartbeatScheduler.Stop()

	if metricsServer != nil {
		metricsShutdownCtx, metricsShutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := metricsServer.Shutdown(metricsShutdownCtx); err != nil {
			slog.Error("metrics server forced to shutdown", "error", err)
		}
		metricsShutdownCancel()
	}
	slog.Info("server stopped")
}
