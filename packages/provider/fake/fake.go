// Package fake provides a scriptable provider client for tests and early development.
package fake

import (
	"context"
	"time"

	"erasmus/packages/provider"
)

// Client streams a fixed script of provider events.
type Client struct {
	NameValue string
	Script    []provider.Event
	Delay     time.Duration
	Requests  []provider.Request
}

// Name returns the provider name.
func (c *Client) Name() string {
	if c.NameValue != "" {
		return c.NameValue
	}
	return "fake"
}

// Stream records the request and streams scripted events until done or canceled.
func (c *Client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	c.Requests = append(c.Requests, req)
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		for _, ev := range c.Script {
			if c.Delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(c.Delay):
				}
			}
			select {
			case <-ctx.Done():
				return
			case out <- ev:
			}
		}
	}()
	return out, nil
}

// StreamFunc returns c.Stream as a provider.StreamFunc.
func (c *Client) StreamFunc() provider.StreamFunc {
	return c.Stream
}
