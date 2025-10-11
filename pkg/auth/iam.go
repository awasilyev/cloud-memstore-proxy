package auth

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// IAMTokenProvider provides GCP IAM tokens for authentication
type IAMTokenProvider struct {
	tokenSource oauth2.TokenSource
}

// NewIAMTokenProvider creates a new IAM token provider
func NewIAMTokenProvider(ctx context.Context) (*IAMTokenProvider, error) {
	// Get default credentials with cloud-platform scope
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to get default credentials: %w", err)
	}

	return &IAMTokenProvider{
		tokenSource: creds.TokenSource,
	}, nil
}

// GetToken returns a fresh IAM token
func (p *IAMTokenProvider) GetToken(ctx context.Context) (string, error) {
	token, err := p.tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}
	return token.AccessToken, nil
}
