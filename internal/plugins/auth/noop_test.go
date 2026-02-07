package auth

import (
	"context"
	"testing"

	"github.com/rjsadow/launchpad/internal/plugins"
)

func TestNewNoopAuthProvider(t *testing.T) {
	p := NewNoopAuthProvider()
	if p == nil {
		t.Fatal("NewNoopAuthProvider returned nil")
	}
	if p.Name() != "noop" {
		t.Errorf("expected name 'noop', got %q", p.Name())
	}
	if p.Type() != plugins.PluginTypeAuth {
		t.Errorf("expected type %q, got %q", plugins.PluginTypeAuth, p.Type())
	}
	if p.Version() == "" {
		t.Error("expected non-empty version")
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestNoopInitialize(t *testing.T) {
	p := NewNoopAuthProvider()

	t.Run("nil config", func(t *testing.T) {
		err := p.Initialize(context.Background(), nil)
		if err != nil {
			t.Errorf("Initialize should accept nil config, got: %v", err)
		}
	})

	t.Run("empty config", func(t *testing.T) {
		err := p.Initialize(context.Background(), map[string]string{})
		if err != nil {
			t.Errorf("Initialize should accept empty config, got: %v", err)
		}
	})

	t.Run("arbitrary config", func(t *testing.T) {
		cfg := map[string]string{"key": "value"}
		err := p.Initialize(context.Background(), cfg)
		if err != nil {
			t.Errorf("Initialize should accept any config, got: %v", err)
		}
	})
}

func TestNoopHealthy(t *testing.T) {
	p := NewNoopAuthProvider()
	if !p.Healthy(context.Background()) {
		t.Error("noop provider should always be healthy")
	}
}

func TestNoopAuthenticate(t *testing.T) {
	p := NewNoopAuthProvider()

	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"arbitrary token", "some-token"},
		{"jwt-like token", "eyJ.eyJ.sig"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := p.Authenticate(context.Background(), tc.token)
			if err != nil {
				t.Fatalf("Authenticate returned error: %v", err)
			}
			if !result.Authenticated {
				t.Error("noop provider should always authenticate")
			}
			if result.User == nil {
				t.Fatal("expected non-nil user")
			}
			if result.User.ID != "anonymous" {
				t.Errorf("expected user ID 'anonymous', got %q", result.User.ID)
			}
			if result.User.Username != "anonymous" {
				t.Errorf("expected username 'anonymous', got %q", result.User.Username)
			}
		})
	}
}

func TestNoopGetUser(t *testing.T) {
	p := NewNoopAuthProvider()

	t.Run("returns user with given ID", func(t *testing.T) {
		user, err := p.GetUser(context.Background(), "user-123")
		if err != nil {
			t.Fatalf("GetUser returned error: %v", err)
		}
		if user == nil {
			t.Fatal("expected non-nil user")
		}
		if user.ID != "user-123" {
			t.Errorf("expected user ID 'user-123', got %q", user.ID)
		}
		if user.Username != "user-123" {
			t.Errorf("expected username 'user-123', got %q", user.Username)
		}
	})

	t.Run("returns user role", func(t *testing.T) {
		user, err := p.GetUser(context.Background(), "test")
		if err != nil {
			t.Fatalf("GetUser returned error: %v", err)
		}
		if len(user.Roles) != 1 || user.Roles[0] != "user" {
			t.Errorf("expected roles [user], got %v", user.Roles)
		}
	})
}

func TestNoopHasPermission(t *testing.T) {
	p := NewNoopAuthProvider()

	tests := []struct {
		name       string
		userID     string
		permission string
	}{
		{"any user any permission", "user-1", "admin"},
		{"empty user", "", "read"},
		{"empty permission", "user-1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			has, err := p.HasPermission(context.Background(), tc.userID, tc.permission)
			if err != nil {
				t.Fatalf("HasPermission returned error: %v", err)
			}
			if !has {
				t.Error("noop provider should always grant permission")
			}
		})
	}
}

func TestNoopGetLoginURL(t *testing.T) {
	p := NewNoopAuthProvider()

	t.Run("returns redirect URL unchanged", func(t *testing.T) {
		url := p.GetLoginURL("/dashboard")
		if url != "/dashboard" {
			t.Errorf("expected '/dashboard', got %q", url)
		}
	})

	t.Run("empty redirect", func(t *testing.T) {
		url := p.GetLoginURL("")
		if url != "" {
			t.Errorf("expected empty string, got %q", url)
		}
	})
}

func TestNoopHandleCallback(t *testing.T) {
	p := NewNoopAuthProvider()

	result, err := p.HandleCallback(context.Background(), "code", "state")
	if err != nil {
		t.Fatalf("HandleCallback returned error: %v", err)
	}
	if !result.Authenticated {
		t.Error("noop HandleCallback should return authenticated result")
	}
	if result.User == nil {
		t.Fatal("expected non-nil user")
	}
	if result.User.ID != "anonymous" {
		t.Errorf("expected user ID 'anonymous', got %q", result.User.ID)
	}
}

func TestNoopLogout(t *testing.T) {
	p := NewNoopAuthProvider()

	err := p.Logout(context.Background(), "any-token")
	if err != nil {
		t.Errorf("Logout should return nil, got: %v", err)
	}
}

func TestNoopClose(t *testing.T) {
	p := NewNoopAuthProvider()

	err := p.Close()
	if err != nil {
		t.Errorf("Close should return nil, got: %v", err)
	}
}

func TestNoopInterfaceCompliance(t *testing.T) {
	// Verify NoopAuthProvider satisfies AuthProvider at compile time
	var _ plugins.AuthProvider = (*NoopAuthProvider)(nil)
}
