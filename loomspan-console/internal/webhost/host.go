package webhost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

type ListenFunc func(network, address string) (net.Listener, error)

type Host struct {
	Address     string
	Handler     http.Handler
	Prepare     func(Authority) (http.Handler, error)
	PreShutdown func(context.Context) error
	Listen      ListenFunc
	OnListen    func(net.Addr)
}

func ValidateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listener address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		return fmt.Errorf("listener address %q must use the exact IPv4 loopback IP 127.0.0.1", address)
	}
	return nil
}

func (host Host) Run(runContext context.Context) error {
	if err := ValidateLoopbackAddress(host.Address); err != nil {
		return err
	}
	if host.Handler == nil && host.Prepare == nil {
		return fmt.Errorf("HTTP handler is required")
	}
	listen := host.Listen
	if listen == nil {
		listen = net.Listen
	}
	listener, err := listen("tcp", host.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", host.Address, err)
	}
	handler := host.Handler
	if host.Prepare != nil {
		authority, err := AuthorityFromAddress(listener.Addr())
		if err != nil {
			listener.Close()
			return err
		}
		handler, err = host.Prepare(authority)
		if err != nil {
			listener.Close()
			return err
		}
		if handler == nil {
			listener.Close()
			return fmt.Errorf("prepared HTTP handler is required")
		}
	}
	if host.OnListen != nil {
		host.OnListen(listener.Addr())
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	result := make(chan error, 1)
	go func() {
		result <- server.Serve(listener)
	}()
	select {
	case <-runContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if host.PreShutdown != nil {
			if err := host.PreShutdown(shutdownContext); err != nil {
				_ = server.Close()
				<-result
				return fmt.Errorf("prepare Console host shutdown: %w", err)
			}
		}
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shut down Console host: %w", err)
		}
		err := <-result
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		cause := context.Cause(runContext)
		if errors.Is(cause, context.Canceled) {
			return nil
		}
		return cause
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
