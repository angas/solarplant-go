package www

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/angas/solarplant-go/config"
	"github.com/angas/solarplant-go/database"
	"github.com/angas/solarplant-go/ferroamp"
	"github.com/angas/solarplant-go/task"
)

type Server struct {
	logger      *slog.Logger
	db          *database.Database
	fa          *ferroamp.FaInMemData
	hub         *Hub
	recentHours *database.RecentHours
	config      *config.AppConfig
	tm          *TemplateManager
}

//go:embed static
var embeddedStaticDir embed.FS

func StartServer(
	db *database.Database,
	tasks *task.Tasks,
	faInMem *ferroamp.FaInMemData,
	recentHours *database.RecentHours,
	cnfg *config.AppConfig,
	currentVersion string) *Server {

	logger := slog.Default().With("module", "www")
	tm, err := NewTemplateManager(logger, cnfg.Api.WwwDir)
	if err != nil {
		logger.Error("template manager initialization error", slog.Any("error", err))
	}

	s := &Server{
		logger:      logger,
		db:          db,
		fa:          faInMem,
		hub:         NewHub(logger),
		recentHours: recentHours,
		config:      cnfg,
		tm:          tm,
	}

	go s.hub.Run()

	http.Handle("/", staticFilesHandler(cnfg.Api.WwwDir))

	http.Handle("GET /sysinfo", NewSysInfoHandler(
		logger.With(slog.String("handler", "timeseries")),
		s.tm,
		currentVersion))

	http.Handle("GET /timeseries", NewTimeSeriesHandler(
		logger.With(slog.String("handler", "timeseries")),
		s.db,
		s.tm,
		s.recentHours,
	))

	http.Handle("GET /dailystats", NewDailyStatsHandler(
		logger.With(slog.String("handler", "timeseries")),
		s.db,
		s.tm,
	))

	http.Handle("GET /log", NewLogHandler(logger.With(
		slog.String("handler", "log")),
		s.config.Api,
		s.db,
		s.tm))

	http.Handle("GET /chart", NewChartHandler(
		logger.With(slog.String("handler", "chart")),
		s.db))

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		name := r.Header.Get("User-Agent")
		client, err := NewClient(s.hub, w, r, name)
		if err != nil {
			s.logger.Error("new websocket client failed", slog.Any("error", err))
			return
		}

		s.hub.Register <- client
		go client.WritePump()
	})

	return s
}

func (s *Server) Run(ctx context.Context) {
	s.logger.Info("staring server...", "port", s.config.Api.Port)

	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", s.config.Api.Port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.logger.Debug("http request",
				slog.String("method", r.Method),
				slog.String("url", r.URL.String()),
				slog.String("remoteAddr", r.RemoteAddr))

			http.DefaultServeMux.ServeHTTP(w, r)
		}),
	}

	srvErrors := make(chan error, 1)

	go func() {
		srvErrors <- srv.ListenAndServe()
	}()

	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()

	// Keeping state to avoid spamming logs
	realTimeErrorState := false
	realTimeMgr := NewRealTimeManager(s.db, s.fa, s.recentHours, s.config.EnergyPrice)

	for {
		select {
		case err := <-srvErrors:
			if err != nil {
				s.logger.Error("server error", slog.Any("error", err))
			}

		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()
			err := srv.Shutdown(shutdownCtx)
			if err != nil {
				s.logger.Error("server shutdown failed", slog.Any("error", err))
			}
			return

		case <-ticker.C:
			rtData, err := realTimeMgr.Get(ctx)
			if err != nil {
				if !realTimeErrorState {
					s.logger.Error("failed to get real time data", slog.Any("error", err))
					realTimeErrorState = true
				}
				continue
			} else {
				realTimeErrorState = false
			}

			buf, err := s.tm.Execute("real_time_data.html", rtData)
			if err != nil {
				s.logger.Error("template execution failed", slog.Any("error", err))
				return
			}

			s.hub.Broadcast <- buf.Bytes()
		}
	}
}

func staticFilesHandler(extDir *string) http.Handler {
	if extDir != nil && *extDir != "" {
		staticDir := path.Join(*extDir, "static")
		if _, err := os.Stat(staticDir); err == nil {
			return http.FileServer(http.Dir(staticDir))
		}
	}

	fsys, err := fs.Sub(embeddedStaticDir, "static")
	if err != nil {
		panic(fmt.Sprintf("failed to create sub filesystem: %v", err))
	}
	return http.FileServer(http.FS(fsys))
}
