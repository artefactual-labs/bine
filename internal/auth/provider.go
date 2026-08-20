package auth

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

const githubClientID = "Ov23ligoVhIvMf0h4b7R"

// Provider describes the OAuth and identity endpoints for an authenticated
// service host.
type Provider struct {
	Name         string
	Host         string
	ClientID     string
	Endpoint     oauth2.Endpoint
	IdentityURL  string
	AccountField string
}

// OAuthConfig returns the provider's OAuth client configuration.
func (p Provider) OAuthConfig() oauth2.Config {
	return oauth2.Config{
		ClientID: p.ClientID,
		Endpoint: p.Endpoint,
	}
}

// GitHubProvider returns the provider configuration for github.com.
func GitHubProvider() Provider {
	endpoint := endpoints.GitHub
	endpoint.AuthStyle = oauth2.AuthStyleInParams

	return Provider{
		Name:         "GitHub",
		Host:         "github.com",
		ClientID:     githubClientID,
		Endpoint:     endpoint,
		IdentityURL:  "https://api.github.com/user",
		AccountField: "login",
	}
}

// Registry maps service hosts to their authentication providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates a provider registry.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		r.providers[strings.ToLower(provider.Host)] = provider
	}

	return r
}

// DefaultRegistry creates the registry supported by this build of bine.
func DefaultRegistry() *Registry {
	return NewRegistry(GitHubProvider())
}

// Lookup returns the provider registered for host.
func (r *Registry) Lookup(host string) (Provider, error) {
	normalized, err := normalizeHost(host)
	if err != nil {
		return Provider{}, err
	}

	provider, ok := r.providers[normalized]
	if !ok {
		return Provider{}, fmt.Errorf("authentication for %q is not supported", normalized)
	}

	return provider, nil
}

// Providers returns all registered providers ordered by host.
func (r *Registry) Providers() []Provider {
	providers := make([]Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Host < providers[j].Host
	})

	return providers
}

func normalizeHost(host string) (string, error) {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return "", fmt.Errorf("host cannot be empty")
	}

	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err != nil {
			return "", fmt.Errorf("parse host: %w", err)
		}
		if u.Hostname() == "" || u.User != nil || u.Port() != "" ||
			(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return "", fmt.Errorf("invalid authentication host %q", host)
		}
		host = u.Hostname()
	}

	if strings.ContainsAny(host, "/?#") {
		return "", fmt.Errorf("invalid authentication host %q", host)
	}

	return host, nil
}
