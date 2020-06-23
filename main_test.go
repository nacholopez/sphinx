package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/nacholopez/sphinx/config"
	"github.com/nacholopez/sphinx/daemon"
	"github.com/nacholopez/sphinx/handlers"
	"github.com/nacholopez/sphinx/ratelimiter"
)

var host = "http://localhost:8081"

type Handler struct{}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte{})
}

func setUpLocalServer() {
	go http.ListenAndServe(":8081", Handler{})
}

func setUpHTTPLimiter(b *testing.B) {
	config, err := config.New("example.yaml")
	if err != nil {
		b.Fatalf("LOAD_CONFIG_FAILED: %s", err.Error())
	}
	rateLimiter, err := ratelimiter.New(config)
	if err != nil {
		b.Fatalf("SPHINX_INIT_FAILED: %s", err.Error())
	}

	// if configuration says that use http
	if config.Proxy.Handler != "http" {
		b.Fatalf("sphinx only supports the http handler")
	}

	// ignore the url in the config and use localhost
	target, _ := url.Parse(host)
	proxy := httputil.NewSingleHostReverseProxy(target)
	httpLimiter := handlers.NewHTTPLimiter(rateLimiter, proxy, false)

	go http.ListenAndServe(":8082", httpLimiter)
}

func makeRequestTo(port string) error {
	// Add basic auth so that we match some buckets.
	if resp, err := http.Get("http://user:pass@localhost" + port); err != nil {
		log.Printf("got resp %#v", resp)
		return err
	}
	return nil
}

func BenchmarkNoLimiter(b *testing.B) {
	setUpLocalServer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := makeRequestTo(":8081"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReasonableConfig(b *testing.B) {
	setUpLocalServer()
	setUpHTTPLimiter(b)
	// So we don't spam with logs
	log.SetOutput(ioutil.Discard)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		makeRequestTo(":8082")
	}
}

func TestSighupHandler(t *testing.T) {
	ranHandler := make(chan bool, 1)
	handler := func(d daemon.Daemon) {
		ranHandler <- true
	}
	conf, _ := config.New("example.yaml")
	d, _ := daemon.New(conf)
	setupSighupHandler(d, handler)
	// Need to sleep here so that the goroutine has time to set up the signal listener, otherwise
	// the signal gets missed
	time.Sleep(1 * time.Second)
	syscall.Kill(syscall.Getpid(), syscall.SIGHUP)

	// Give the syscall 1 second to be handled, should be more than enough time
	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(time.Duration(1 * time.Second))
		timeout <- true
	}()
	select {
	case <-ranHandler:
	case <-timeout:
		t.Fatal("Didn't run handler")
	}

	// Try calling again and make sure it still happens
	syscall.Kill(syscall.Getpid(), syscall.SIGHUP)
	select {
	case <-ranHandler:
	case <-timeout:
		t.Fatal("Didn't run handler second time")
	}
}
