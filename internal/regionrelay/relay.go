package regionrelay

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// Relay is a started relay: its forwards and its cache.
type Relay struct {
	forwards []*Forward
	cache    *http.Server
	cacheLn  net.Listener
	logger   *slog.Logger
}

// Start binds every listener of config. Nothing runs until Serve; on an
// error every listener already bound is closed.
func Start(config *Config, logger *slog.Logger) (*Relay, error) {
	if logger == nil {
		logger = slog.Default()
	}
	relay := &Relay{logger: logger}
	for _, forwardConfig := range config.Forwards {
		forward, err := ListenForward(forwardConfig, logger)
		if err != nil {
			relay.close()
			return nil, err
		}
		relay.forwards = append(relay.forwards, forward)
	}
	if config.Cache != nil {
		cache, err := NewCache(*config.Cache, logger)
		if err != nil {
			relay.close()
			return nil, fmt.Errorf("cache: %w", err)
		}
		certificates, err := newCertificateReloader(config.Cache.TLSCertFile, config.Cache.TLSKeyFile)
		if err != nil {
			relay.close()
			return nil, fmt.Errorf("cache: %w", err)
		}
		listener, err := net.Listen("tcp", config.Cache.Listen)
		if err != nil {
			relay.close()
			return nil, fmt.Errorf("cache: %w", err)
		}
		relay.cacheLn = listener
		relay.cache = &http.Server{
			Handler:           cache,
			TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: certificates.get},
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
			MaxHeaderBytes:    16 << 10,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		}
	}
	return relay, nil
}

// ForwardAddrs are the bound forward addresses, in configuration order.
func (r *Relay) ForwardAddrs() []net.Addr {
	addrs := make([]net.Addr, 0, len(r.forwards))
	for _, forward := range r.forwards {
		addrs = append(addrs, forward.Addr())
	}
	return addrs
}

// CacheAddr is the bound cache address, nil without a cache.
func (r *Relay) CacheAddr() net.Addr {
	if r.cacheLn == nil {
		return nil
	}
	return r.cacheLn.Addr()
}

// Serve runs until ctx ends or a listener fails, then closes everything.
func (r *Relay) Serve(ctx context.Context) error {
	errs := make(chan error, len(r.forwards)+1)
	var wg sync.WaitGroup
	for _, forward := range r.forwards {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := forward.Serve(); err != nil {
				errs <- err
			}
		}()
	}
	if r.cache != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.logger.Info("relay cache listening", "listen", r.cacheLn.Addr().String())
			if err := r.cache.ServeTLS(r.cacheLn, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errs <- fmt.Errorf("cache: %w", err)
			}
		}()
	}
	var err error
	select {
	case <-ctx.Done():
	case err = <-errs:
	}
	r.close()
	wg.Wait()
	return err
}

func (r *Relay) close() {
	for _, forward := range r.forwards {
		_ = forward.Close()
	}
	if r.cache != nil {
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = r.cache.Shutdown(shutdown)
		cancel()
	} else if r.cacheLn != nil {
		_ = r.cacheLn.Close()
	}
}
