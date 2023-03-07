package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"rest-queue/storage"
)

const (
	queryKey    = "v"
	timeoutKey  = "timeout"
	defaultPort = "8080"
)

type appEnv struct {
	port string
}

func (app *appEnv) fromArgs(args []string) error {
	fl := flag.NewFlagSet("rest-queue", flag.ContinueOnError)
	fl.StringVar(&app.port, "port", defaultPort, "a port to run rest api on")
	return fl.Parse(args)
}

func main() {
	var app appEnv
	err := app.fromArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	s := storage.New()
	api := &api{
		s,
		make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(api.queryHandler))
	srv := &http.Server{
		Addr:        fmt.Sprintf(":%s", app.port),
		Handler:     mux,
		ReadTimeout: time.Duration(10) * time.Second,
		IdleTimeout: time.Duration(10) * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("\nreceived interrupt signal, stopping server")
	close(api.cancelCh)
	timeout, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()
	if err := srv.Shutdown(timeout); err != nil {
		log.Printf("can't shutdown http server: %s", err)
	}
}

type api struct {
	storage  *storage.Store
	cancelCh chan struct{}
}

func (a *api) queryHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		a.putInQueue(w, r)
	case http.MethodGet:
		a.getFromQueue(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// PUT /{pet}?v=cat - stores item in pet queue
func (a *api) putInQueue(w http.ResponseWriter, r *http.Request) {
	queueName := strings.TrimPrefix(r.URL.Path, "/")

	values := r.URL.Query()
	if !values.Has(queryKey) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	value := values.Get(queryKey)
	a.storage.Store(queueName, value)

	w.WriteHeader(http.StatusOK)
}

// GET /{pet}?timeout=N - returns item from pet queue
func (a *api) getFromQueue(w http.ResponseWriter, r *http.Request) {
	queueName := strings.TrimPrefix(r.URL.Path, "/")
	values := r.URL.Query()

	result := a.storage.Get(queueName)

	if result == "" && values.Has(timeoutKey) {
		ctx := r.Context()
		secStr := values.Get(timeoutKey)
		dur, err := strconv.Atoi(secStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		ch := make(chan string)
		elem := a.storage.AddToWaitQueue(queueName, ch)
		defer a.storage.RemoveFromWaitQueue(queueName, elem)
		reqTimer := time.NewTimer(time.Duration(dur) * time.Second)
		defer reqTimer.Stop()

		for {
			select {
			case item := <-ch:
				log.Println("got item")
				w.Write([]byte(item))
				return
			case <-reqTimer.C:
				log.Println("time is out")
				w.WriteHeader(http.StatusNotFound)
				return
			case <-ctx.Done():
				log.Printf("request canceled by client, cause: %s\n", context.Cause(ctx))
				return
			case <-a.cancelCh:
				log.Println("received signal to stop server")
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("server closed"))
				return
			}
		}
	}

	if result == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Write([]byte(result))
}
