package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
	"xiaotao/xt-client/internal/app"
	"xiaotao/xt-client/internal/desktop"
	"xiaotao/xt-client/internal/logging"
	"xiaotao/xt-client/internal/runtimeenv"
)

func main() {
	environment := runtimeenv.Current()
	if e := run(); e != nil {
		log.Print(e)
		if !hasServiceFlag() {
			desktop.Alert(environment.DisplayName, e.Error())
		}
		os.Exit(1)
	}
}

func hasServiceFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--service" {
			return true
		}
	}
	return false
}

func run() error {
	data := flag.String("data-dir", "", "用户数据目录")
	service := flag.Bool("service", false, "运行服务")
	window := flag.Bool("window", false, "打开桌面窗口")
	hudWindow := flag.Bool("hud-window", false, "打开独立请求浮窗")
	browser := flag.Bool("browser", false, "在浏览器打开")
	status := flag.Bool("status", false, "显示服务地址")
	stop := flag.Bool("stop", false, "停止服务")
	flag.Parse()

	environment := runtimeenv.Current()
	dataDir, err := resolveDataDir(*data)
	if err != nil {
		return err
	}

	set, clientLogger, err := initLogging(dataDir, environment)
	if err != nil {
		return err
	}
	defer func() {
		_ = set.Close()
		logging.SetDefault(nil)
	}()

	if *service {
		return runService(dataDir, clientLogger)
	}
	handled, err := handleStopOrStatus(dataDir, *stop, *status)
	if handled {
		return err
	}
	return launchClient(dataDir, *browser, *window, *hudWindow, environment, clientLogger)
}

func resolveDataDir(custom string) (string, error) {
	if custom == "" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", fmt.Errorf("LOCALAPPDATA unavailable")
		}
		custom = runtimeenv.DefaultDataDir(base)
	}
	abs, err := filepath.Abs(custom)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(abs)
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return "", err
	}
	return clean, nil
}

func initLogging(dataDir string, env runtimeenv.Environment) (*logging.Set, *logging.Logger, error) {
	rawLevel := os.Getenv("XIAOTAO_LOG_LEVEL")
	level := logging.LevelInfo
	var levelErr error
	if rawLevel != "" {
		level, levelErr = logging.ParseLevel(rawLevel)
	}
	set, err := logging.Open(filepath.Join(dataDir, "logs"), level)
	if err != nil {
		return nil, nil, err
	}
	logging.SetDefault(set)
	clientLogger := set.For(logging.ModuleClient)
	log.SetOutput(clientLogger)

	if levelErr != nil {
		clientLogger.Warn("invalid XIAOTAO_LOG_LEVEL, fallback to info", logging.F("val", rawLevel), logging.F("error", levelErr))
	}
	if _, _, cleanErr := logging.Cleanup(filepath.Join(dataDir, "logs"), 14); cleanErr != nil {
		clientLogger.Warn("logs cleanup failed", logging.F("error", cleanErr))
	}
	clientLogger.Info("process started",
		logging.F("env", env.Name),
		logging.F("data_dir", dataDir),
		logging.F("pid", os.Getpid()),
		logging.F("args", strings.Join(os.Args[1:], " ")),
	)
	return set, clientLogger, nil
}

func runService(dataDir string, logger *logging.Logger) error {
	if current, readErr := app.Read(dataDir); readErr == nil && app.Alive(current) {
		logger.Info("service already running, exiting", logging.F("mode", "service"))
		return nil
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	logger.Info("service starting", logging.F("data_dir", dataDir))
	err := app.Serve(ctx, dataDir)
	if err != nil {
		logger.Error("service exited with error", logging.F("error", err))
	} else {
		logger.Info("service exited cleanly")
	}
	return err
}

func handleStopOrStatus(dataDir string, stop, status bool) (bool, error) {
	if !stop && !status {
		return false, nil
	}
	s, e := app.Read(dataDir)
	if e != nil || !app.Alive(s) {
		return true, fmt.Errorf("服务未运行")
	}
	if status {
		fmt.Println(s.URL)
		return true, nil
	}
	req, _ := http.NewRequest("POST", s.URL+"/api/shutdown", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token)
	c := http.Client{Timeout: 3 * time.Second}
	r, e := c.Do(req)
	if e != nil {
		return true, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return true, fmt.Errorf("shutdown: %s", r.Status)
	}
	return true, nil
}

func launchClient(dataDir string, browser, window, hudWindow bool, env runtimeenv.Environment, logger *logging.Logger) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	s, err := ensureService(exe, dataDir, logger)
	if err != nil {
		return err
	}
	if hudWindow {
		release, lockErr := desktop.Lock(runtimeenv.HUDWindowMutex(app.Key(dataDir)))
		if lockErr != nil {
			return nil
		}
		defer release()
		return desktop.HUDWindow(s.URL, dataDir, s.Token, env.DisplayName)
	}
	return openClient(browser, window, exe, dataDir, s, env, logger)
}

func ensureService(exe, dataDir string, logger *logging.Logger) (app.State, error) {
	s, e := app.Read(dataDir)
	if app.Alive(s) {
		return s, nil
	}
	release, lockErr := desktop.Lock(runtimeenv.LaunchMutex(app.Key(dataDir)))
	if lockErr != nil {
		s, e = app.Read(dataDir)
		if e == nil && app.Alive(s) {
			return s, nil
		}
		return s, lockErr
	}
	defer release()

	s, e = app.Read(dataDir)
	if e == nil && app.Alive(s) {
		return s, nil
	}
	if e = desktop.Start(exe, "--service", "--data-dir", dataDir); e != nil {
		logger.Error("desktop start service failed", logging.F("error", e))
		return s, e
	}
	for i := 0; i < 100; i++ {
		time.Sleep(100 * time.Millisecond)
		s, e = app.Read(dataDir)
		if e == nil && app.Alive(s) {
			logger.Info("service started successfully", logging.F("url", s.URL))
			return s, nil
		}
	}
	logger.Error("service start timeout", logging.F("error", "timeout waiting for service"))
	return s, fmt.Errorf("服务启动超时，请查看 %s", logger.Path())
}

func openClient(
	browser bool,
	window bool,
	exe string,
	data string,
	s app.State,
	environment runtimeenv.Environment,
	logger *logging.Logger,
) error {
	if browser {
		return desktop.Browser(s.URL)
	}
	if window {
		release, e := desktop.Lock(runtimeenv.WindowMutex(app.Key(data)))
		if e != nil {
			if desktop.ShowExisting(environment.DisplayName) {
				return nil
			}
			return desktop.Browser(s.URL)
		}
		defer release()
		if e = desktop.Window(s.URL, data, s.Token, environment.DisplayName); e != nil {
			logger.Warn("desktop window failed, fallback to browser", logging.F("error", e))
			log.Print(e)
			return desktop.Browser(s.URL)
		}
		return nil
	}
	if e := desktop.Start(exe, "--window", "--data-dir", data); e != nil {
		logger.Error("desktop start window failed", logging.F("error", e))
		return e
	}
	return nil
}
