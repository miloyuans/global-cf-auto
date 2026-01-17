package app

import (
	"context"
	"errors"

	"DomainC/tools"
)

type DefaultWhoisClient struct{}

var ErrWhoisExpiryNotFound = errors.New("expiry lookup failed")

func (DefaultWhoisClient) Query(ctx context.Context, domain string) (string, error) {
	type result struct {
		data string
		err  error
	}

	ch := make(chan result, 1)

	go func() {
		expiry, ok := tools.CheckWhois(domain)
		if !ok {
			ch <- result{data: "", err: ErrWhoisExpiryNotFound}
			return
		}
		ch <- result{data: expiry, err: nil}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		return res.data, res.err
	}
}
