package repo

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/net/proxy"
)

// ConfigureDatabaseProxy is optional development transport configuration. It
// applies equally to goose and the runtime pool, without changing database URLs.
func ConfigureDatabaseProxy(config *pgx.ConnConfig, proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil || (u.Scheme != "socks5" && u.Scheme != "socks5h") || u.Host == "" {
		return errors.New("database proxy must be a socks5 URL")
	}
	u.Scheme = "socks5"
	d, err := proxy.FromURL(u, &net.Dialer{Timeout: 10 * time.Second})
	if err != nil {
		return errors.New("invalid database proxy configuration")
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		return errors.New("database proxy lacks cancellation support")
	}
	config.DialFunc = cd.DialContext
	config.LookupFunc = func(_ context.Context, host string) ([]string, error) { return []string{host}, nil }
	return nil
}

func OpenDatabase(databaseURL, proxyURL string) (*sql.DB, error) {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	if err = ConfigureDatabaseProxy(config, proxyURL); err != nil {
		return nil, err
	}
	return stdlib.OpenDB(*config), nil
}
