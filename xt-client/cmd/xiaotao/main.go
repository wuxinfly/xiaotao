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
	"time"
	"xiaotao/xt-client/internal/app"
	"xiaotao/xt-client/internal/desktop"
)

func main() {
	if e := run(); e != nil {
		log.Print(e)
		if !hasServiceFlag() {
			desktop.Alert(e.Error())
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
	browser := flag.Bool("browser", false, "在浏览器打开")
	status := flag.Bool("status", false, "显示服务地址")
	stop := flag.Bool("stop", false, "停止服务")
	flag.Parse()
	if *data == "" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return fmt.Errorf("LOCALAPPDATA unavailable")
		}
		*data = filepath.Join(base, "XiaoTao")
	}
	abs, e := filepath.Abs(*data)
	if e != nil {
		return e
	}
	*data = filepath.Clean(abs)
	if e = os.MkdirAll(*data, 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(*data, "client.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	log.SetOutput(f)
	if *service {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		return app.Serve(ctx, *data)
	}
	s, e := app.Read(*data)
	if *stop || *status {
		if e != nil || !app.Alive(s) {
			return fmt.Errorf("服务未运行")
		}
		if *status {
			fmt.Println(s.URL)
			return nil
		}
		req, _ := http.NewRequest("POST", s.URL+"/api/shutdown", nil)
		req.Header.Set("Authorization", "Bearer "+s.Token)
		c := http.Client{Timeout: 3 * time.Second}
		r, e := c.Do(req)
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("shutdown: %s", r.Status)
		}
		return nil
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if e != nil || !app.Alive(s) {
		if e = desktop.Start(exe, "--service", "--data-dir", *data); e != nil {
			return e
		}
		ready := false
		for i := 0; i < 100; i++ {
			time.Sleep(100 * time.Millisecond)
			s, e = app.Read(*data)
			if e == nil && app.Alive(s) {
				ready = true
				break
			}
		}
		if !ready {
			return fmt.Errorf("服务启动超时，请查看 client.log")
		}
	}
	if *browser {
		return desktop.Browser(s.URL)
	}
	if *window {
		release, e := desktop.Lock("Local\\XiaoTao.Window." + app.Key(*data))
		if e != nil {
			if desktop.ShowExisting() {
				return nil
			}
			return desktop.Browser(s.URL)
		}
		defer release()
		if e = desktop.Window(s.URL, *data, s.Token); e != nil {
			log.Print(e)
			return desktop.Browser(s.URL)
		}
		return nil
	}
	return desktop.Start(exe, "--window", "--data-dir", *data)
}
