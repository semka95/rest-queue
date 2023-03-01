package main

import (
	"log"
	"net/http"
	"rest-queue/storage"
	"strconv"
	"strings"
	"time"
)

const (
	queryKey   = "v"
	timeoutKey = "timeout"
)

type api struct {
	storage *storage.Storage
}

func main() {
	storage := storage.New()
	api := &api{
		storage,
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(api.queryHandler))

	srv := &http.Server{
		Addr:        ":8080", // TODO: add setting port from arguments
		Handler:     mux,
		ReadTimeout: time.Duration(10) * time.Second,
		IdleTimeout: time.Duration(10) * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
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

		elem := a.storage.GetChan(queueName)
		defer a.storage.RemoveFromWait(queueName, elem)
		ch := elem.Value.(chan string)
		defer close(ch)

		timer := time.NewTimer(time.Duration(dur) * time.Second)
		for {
			select {
			case result := <-ch: // don't need to check if channel closed, because after two possible closes in this function it returns
				log.Println("got item")
				w.Write([]byte(result))
				return
			case <-timer.C:
				log.Println("time is out")
				w.WriteHeader(http.StatusNotFound)
				return
			case <-ctx.Done():
				log.Println("request canceled by client")
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
