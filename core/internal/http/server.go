package httpserver

import (
	"net/http"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/config"
)

func NewServer(cfg config.ServerConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         cfg.Addr(),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
}
