package health

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func SQLPingChecker(db *sql.DB) CheckFunc {
	return func(ctx context.Context) error {
		if db == nil {
			return fmt.Errorf("sql db is nil")
		}
		return db.PingContext(ctx)
	}
}

func RedisPingChecker(ping func(ctx context.Context) error) CheckFunc {
	return func(ctx context.Context) error {
		if ping == nil {
			return fmt.Errorf("redis ping function is nil")
		}
		return ping(ctx)
	}
}

func HTTPReachabilityChecker(client *http.Client, endpoint string) CheckFunc {
	return func(ctx context.Context) error {
		httpClient := client
		if httpClient == nil {
			httpClient = &http.Client{Timeout: 2 * time.Second}
		}
		if strings.TrimSpace(endpoint) == "" {
			return fmt.Errorf("endpoint is empty")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	}
}
