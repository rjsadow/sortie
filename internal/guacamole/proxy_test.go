package guacamole

import (
	"testing"
)

func TestEncodeInstruction(t *testing.T) {
	tests := []struct {
		name   string
		opcode string
		args   []string
		want   string
	}{
		{
			name:   "select rdp",
			opcode: "select",
			args:   []string{"rdp"},
			want:   "6.select,3.rdp;",
		},
		{
			name:   "no args",
			opcode: "audio",
			args:   nil,
			want:   "5.audio;",
		},
		{
			name:   "multiple args",
			opcode: "size",
			args:   []string{"1920", "1080", "96"},
			want:   "4.size,4.1920,4.1080,2.96;",
		},
		{
			name:   "image instruction",
			opcode: "image",
			args:   []string{"image/png", "image/jpeg", "image/webp"},
			want:   "5.image,9.image/png,10.image/jpeg,10.image/webp;",
		},
		{
			name:   "empty arg",
			opcode: "connect",
			args:   []string{"", "value"},
			want:   "7.connect,0.,5.value;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeInstruction(tt.opcode, tt.args...)
			if got != tt.want {
				t.Errorf("encodeInstruction(%q, %v) = %q, want %q", tt.opcode, tt.args, got, tt.want)
			}
		})
	}
}

func TestParseInstruction(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "args with two params",
			raw:  "4.args,8.hostname,4.port;",
			want: []string{"hostname", "port"},
		},
		{
			name: "args with many params",
			raw:  "4.args,8.hostname,4.port,8.username,8.password,5.width,6.height;",
			want: []string{"hostname", "port", "username", "password", "width", "height"},
		},
		{
			name: "single arg",
			raw:  "4.args,8.hostname;",
			want: []string{"hostname"},
		},
		{
			name: "empty input",
			raw:  "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInstruction(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("parseInstruction(%q) returned %d args, want %d: got %v", tt.raw, len(got), len(tt.want), got)
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseInstruction(%q)[%d] = %q, want %q", tt.raw, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestNewGuacdProxy(t *testing.T) {
	proxy := NewGuacdProxy("10.0.0.1:4822", "127.0.0.1", "3389", "user", "pass", "1920", "1080")

	if proxy.guacdAddr != "10.0.0.1:4822" {
		t.Errorf("guacdAddr = %q, want %q", proxy.guacdAddr, "10.0.0.1:4822")
	}
	if proxy.hostname != "127.0.0.1" {
		t.Errorf("hostname = %q, want %q", proxy.hostname, "127.0.0.1")
	}
	if proxy.port != "3389" {
		t.Errorf("port = %q, want %q", proxy.port, "3389")
	}
	if proxy.username != "user" {
		t.Errorf("username = %q, want %q", proxy.username, "user")
	}
	if proxy.password != "pass" {
		t.Errorf("password = %q, want %q", proxy.password, "pass")
	}
	if proxy.width != "1920" {
		t.Errorf("width = %q, want %q", proxy.width, "1920")
	}
	if proxy.height != "1080" {
		t.Errorf("height = %q, want %q", proxy.height, "1080")
	}
}

func TestBuildConnectArgs(t *testing.T) {
	proxy := NewGuacdProxy("10.0.0.1:4822", "127.0.0.1", "3389", "testuser", "testpass", "1920", "1080")

	argNames := []string{"hostname", "port", "username", "password", "width", "height", "dpi", "unknown-param"}
	result := proxy.buildConnectArgs(argNames)

	expected := []string{"127.0.0.1", "3389", "testuser", "testpass", "1920", "1080", "96", ""}

	if len(result) != len(expected) {
		t.Fatalf("buildConnectArgs returned %d args, want %d", len(result), len(expected))
	}

	for i, v := range result {
		if v != expected[i] {
			t.Errorf("buildConnectArgs()[%d] = %q, want %q (param: %s)", i, v, expected[i], argNames[i])
		}
	}
}

func TestBuildConnectArgs_SecurityParams(t *testing.T) {
	proxy := NewGuacdProxy("10.0.0.1:4822", "127.0.0.1", "3389", "user", "pass", "1920", "1080")

	argNames := []string{"security", "ignore-cert", "disable-auth", "color-depth", "resize-method"}
	result := proxy.buildConnectArgs(argNames)

	expected := []string{"rdp", "true", "true", "24", "display-update"}

	if len(result) != len(expected) {
		t.Fatalf("buildConnectArgs returned %d args, want %d", len(result), len(expected))
	}

	for i, v := range result {
		if v != expected[i] {
			t.Errorf("buildConnectArgs()[%d] = %q, want %q (param: %s)", i, v, expected[i], argNames[i])
		}
	}
}

func TestBuildRDPConnectArgs(t *testing.T) {
	argNames := []string{"hostname", "port", "username", "password", "width", "height", "dpi", "unknown-param"}
	result := buildRDPConnectArgs(argNames, "127.0.0.1", "3389", "testuser", "testpass", "1920", "1080")

	expected := []string{"127.0.0.1", "3389", "testuser", "testpass", "1920", "1080", "96", ""}

	if len(result) != len(expected) {
		t.Fatalf("buildRDPConnectArgs returned %d args, want %d", len(result), len(expected))
	}

	for i, v := range result {
		if v != expected[i] {
			t.Errorf("buildRDPConnectArgs()[%d] = %q, want %q (param: %s)", i, v, expected[i], argNames[i])
		}
	}
}
