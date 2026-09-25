package main

import (
	"context"
	"errors"
	"net"
	"strconv"
)

func (c *Config) configureEventTraces(getenv func(string) string) error {
	addr := getenv("SANDBOX_OTLP_LISTEN_ADDR")
	token := getenv("SANDBOX_OTLP_TOKEN")
	prsURL := getenv("SANDBOX_PRS_URL")
	service := getenv("SANDBOX_TRACE_SERVICE_NAME")
	if addr == "" && token == "" && prsURL == "" && service == "" {
		return nil
	}
	if addr == "" || token == "" || prsURL == "" {
		return errors.New("PRS trace capture requires SANDBOX_OTLP_LISTEN_ADDR, SANDBOX_OTLP_TOKEN and SANDBOX_PRS_URL together")
	}
	_, port, err := net.SplitHostPort(addr)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("SANDBOX_OTLP_LISTEN_ADDR must be a host:port listener address")
	}
	if service == "" {
		service = "nuts-knooppunt"
	}
	bridge, err := newEventTraceBridge(prsURL, service, token)
	if err != nil {
		return err
	}
	c.eventTraces, c.eventTraceListenAddr = bridge, addr
	return nil
}

func withEventTraceCapture(ctx context.Context, bridge *eventTraceBridge, live func() bool) context.Context {
	capture, ok := ctx.Value(eventCaptureKey{}).(eventCapture)
	if !ok || bridge == nil {
		return ctx
	}
	capture.traces, capture.live = bridge, live
	return context.WithValue(ctx, eventCaptureKey{}, capture)
}
