package main

import (
	"errors"
	"time"

	coregrpc "ucupa/ucupa-core/transport/grpc"
)

type transitionApprovalConfig struct {
	MaxTTL       string `json:"max_ttl"`
	MaxApprovals int    `json:"max_approvals"`
	MaxPerCaller int    `json:"max_per_caller"`
}

func (c *transitionApprovalConfig) load(peers bool) (*coregrpc.TransitionApprovalConfig, error) {
	if c == nil {
		return nil, nil
	}
	ttl, err := time.ParseDuration(c.MaxTTL)
	if err != nil || ttl <= 0 || ttl%time.Second != 0 || !peers || c.MaxApprovals <= 0 || c.MaxPerCaller <= 0 || c.MaxPerCaller > c.MaxApprovals {
		return nil, errors.New("invalid transition approval limits or missing peer trust")
	}
	return &coregrpc.TransitionApprovalConfig{MaxTTL: ttl, MaxApprovals: c.MaxApprovals, MaxPerCaller: c.MaxPerCaller}, nil
}

func (c *transitionApprovalConfig) loadCarrier(peers bool) (*coregrpc.CarrierApprovalConfig, error) {
	if c == nil {
		return nil, nil
	}
	ttl, err := time.ParseDuration(c.MaxTTL)
	if err != nil || ttl <= 0 || ttl%time.Second != 0 || !peers || c.MaxApprovals <= 0 || c.MaxPerCaller <= 0 || c.MaxPerCaller > c.MaxApprovals {
		return nil, errors.New("invalid carrier approval limits or missing peer trust")
	}
	return &coregrpc.CarrierApprovalConfig{MaxTTL: ttl, MaxApprovals: c.MaxApprovals, MaxPerCaller: c.MaxPerCaller}, nil
}
