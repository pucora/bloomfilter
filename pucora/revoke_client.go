package pucora

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-contrib/uuid"
	"github.com/pucora/lura/v2/logging"
)

// RevokeClientConfig holds gateway-side revoke server client settings.
type RevokeClientConfig struct {
	PingURL      string
	PingInterval time.Duration
	APIKey       string
	N            uint
	P            float64
	TTL          uint
	HashName     string
	Port         int
}

func parseRevokeClientConfig(cfg Config) (RevokeClientConfig, bool) {
	if cfg.RevokeServerPingURL == "" {
		return RevokeClientConfig{}, false
	}
	interval := 30 * time.Second
	if cfg.RevokeServerPingInterval != "" {
		if d, err := time.ParseDuration(cfg.RevokeServerPingInterval); err == nil {
			interval = d
		}
	}
	return RevokeClientConfig{
		PingURL:      cfg.RevokeServerPingURL,
		PingInterval: interval,
		APIKey:       cfg.RevokeServerAPIKey,
		N:            cfg.N,
		P:            cfg.P,
		TTL:          cfg.TTL,
		HashName:     cfg.HashName,
		Port:         cfg.Port,
	}, true
}

// StartRevokeClient registers this gateway with the revoke server periodically.
func StartRevokeClient(ctx context.Context, cfg Config, logger logging.Logger) {
	clientCfg, ok := parseRevokeClientConfig(cfg)
	if !ok {
		return
	}
	go func() {
		ticker := time.NewTicker(clientCfg.PingInterval)
		defer ticker.Stop()
		register := func() {
			if err := postInstanceRegistration(clientCfg); err != nil {
				logger.Warning("[SERVICE: RevokeClient]", err.Error())
			}
		}
		register()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				register()
			}
		}
	}()
}

func postInstanceRegistration(cfg RevokeClientConfig) error {
	ip := localIP()
	payload := map[string]interface{}{
		"instance_id": uuid.NewV4().String(),
		"cluster_id":  "pucora",
		"cn":          "pucora-ce",
		"n":           cfg.N,
		"p":           cfg.P,
		"ttl":         cfg.TTL,
		"hash_name":   cfg.HashName,
		"ip":          ip,
		"port":        cfg.Port,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, cfg.PingURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "bearer "+cfg.APIKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("revoke server registration failed: %s", resp.Status)
	}
	return nil
}

func localIP() string {
	if v := os.Getenv("POD_IP"); v != "" {
		return v
	}
	return "127.0.0.1"
}

func normalizePingURL(url string) string {
	return strings.TrimRight(url, "/")
}
