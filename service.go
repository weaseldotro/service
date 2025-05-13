package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var registerShutdownChanMutex sync.Mutex

type Service struct {
	Router       http.Handler
	ShutdownFunc func()
	Middleware   func(http.Handler) http.Handler

	server        http.Server
	address       string
	port          int
	isRunning     uint32
	shutdownChans []chan struct{}
	signalChan    chan os.Signal
}

func Init(address string, port int) *Service {
	if address == "" {
		panic("service address cannot be empty on init; use * for all available addresses")
	}

	if address == "*" {
		address = ""
	}

	var service Service

	// service.Router = http.NewServeMux()
	service.port = port
	service.address = address

	service.server = http.Server{
		Addr:         fmt.Sprintf("%s:%d", address, service.port),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      service.Router,
	}

	return &service
}

func (service *Service) RegisterShutdownChan() chan struct{} {
	registerShutdownChanMutex.Lock()
	ch := make(chan struct{})
	service.shutdownChans = append(service.shutdownChans, ch)
	registerShutdownChanMutex.Unlock()
	return ch
}

func (service *Service) UnregisterShutdownChan(ch chan struct{}) {
	registerShutdownChanMutex.Lock()
	for i, c := range service.shutdownChans {
		if c == ch {
			service.shutdownChans = slices.Delete(service.shutdownChans, i, i+1)
			break
		}
	}
	registerShutdownChanMutex.Unlock()
}

func (service *Service) Run() {
	if !atomic.CompareAndSwapUint32(&service.isRunning, 0, 1) {
		return
	}

	if service.Middleware != nil {
		service.server.Handler = service.Middleware(service.Router)
	}

	go func() {
		address := service.address
		if address == "" {
			address = strings.Join(getLocalIPs(), ", ")
		}
		slog.Info("listening to requests", "port", service.port, "address", address)
		if err := service.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("could not listen", "error", err)
			os.Exit(1)
		}
	}()
}

func (service *Service) WaitForStop() {
	service.signalChan = make(chan os.Signal, 1)
	signal.Notify(service.signalChan, syscall.SIGINT, syscall.SIGTERM)

	<-service.signalChan
	slog.Info("received shutdown signal, stopping the service")

	service.Cleanup()
	atomic.StoreUint32(&service.isRunning, 0)
}

func (service *Service) RunAndWait() {
	service.Run()
	service.WaitForStop()
}

func (service *Service) Stop() {
	service.signalChan <- syscall.SIGTERM
}

func (service *Service) Cleanup() {
	for _, ch := range service.shutdownChans {
		close(ch)
	}

	if service.ShutdownFunc != nil {
		service.ShutdownFunc()
	}

	service.server.Shutdown(context.Background())
	slog.Info("service stopped")
}
