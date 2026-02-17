package config

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestHelmChartConfigParity ensures bidirectional parity between config.go's
// SORTIE_* environment variables and the Helm chart templates. It catches:
//  1. New env vars added to config.go but missing from the chart (forward drift)
//  2. Env vars removed from config.go but still lingering in the chart (stale refs)
//  3. Stale entries in either allowlist
func TestHelmChartConfigParity(t *testing.T) {
	// Env vars in config.go intentionally absent from the Helm chart.
	// Every entry MUST have a justification.
	notInChart := map[string]string{
		"SORTIE_PORT": "Container port is hardcoded to 8080 in deployment.yaml",
	}

	// Env vars in the Helm chart that don't have a corresponding os.Getenv()
	// in config.go. Every entry MUST have a justification.
	notInConfig := map[string]string{
		// (none currently — add entries here if needed)
	}

	configVars := extractEnvVarsFromSource(t, "config.go")
	if len(configVars) == 0 {
		t.Fatal("found no SORTIE_* env vars in config.go — extraction is broken")
	}

	chartVars := extractEnvVarsFromChartTemplates(t)
	if len(chartVars) == 0 {
		t.Fatal("found no SORTIE_* env vars in Helm chart templates — extraction is broken")
	}

	configSet := make(map[string]bool, len(configVars))
	for _, v := range configVars {
		configSet[v] = true
	}

	// Forward: config.go vars missing from chart
	var missingFromChart []string
	for _, v := range configVars {
		if _, ok := notInChart[v]; ok {
			continue
		}
		if !chartVars[v] {
			missingFromChart = append(missingFromChart, v)
		}
	}
	if len(missingFromChart) > 0 {
		sort.Strings(missingFromChart)
		t.Errorf("%d env var(s) in config.go missing from Helm chart:", len(missingFromChart))
		for _, v := range missingFromChart {
			t.Errorf("  - %s", v)
		}
		t.Error("Fix: add to charts/sortie/templates/ configmap.yaml or secret.yaml,")
		t.Error("or add to notInChart allowlist with a justification.")
	}

	// Reverse: chart vars no longer in config.go
	var staleInChart []string
	for v := range chartVars {
		if _, ok := notInConfig[v]; ok {
			continue
		}
		if !configSet[v] {
			staleInChart = append(staleInChart, v)
		}
	}
	if len(staleInChart) > 0 {
		sort.Strings(staleInChart)
		t.Errorf("%d env var(s) in Helm chart no longer in config.go:", len(staleInChart))
		for _, v := range staleInChart {
			t.Errorf("  - %s", v)
		}
		t.Error("Fix: remove from charts/sortie/templates/ and values.yaml,")
		t.Error("or add to notInConfig allowlist with a justification.")
	}

	// Verify allowlists don't accumulate stale entries
	for v := range notInChart {
		if !configSet[v] {
			t.Errorf("notInChart entry %q no longer exists in config.go — remove it", v)
		}
	}
	for v := range notInConfig {
		if !chartVars[v] {
			t.Errorf("notInConfig entry %q no longer exists in Helm chart — remove it", v)
		}
	}
}

// extractEnvVarsFromSource reads a Go source file in the current package
// directory and returns all unique SORTIE_* names found in os.Getenv() calls.
func extractEnvVarsFromSource(t *testing.T, filename string) []string {
	t.Helper()

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read %s: %v", filename, err)
	}

	re := regexp.MustCompile(`os\.Getenv\("(SORTIE_[A-Z0-9_]+)"\)`)
	matches := re.FindAllStringSubmatch(string(data), -1)

	seen := make(map[string]bool)
	var vars []string
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			vars = append(vars, name)
		}
	}

	sort.Strings(vars)
	return vars
}

// extractEnvVarsFromChartTemplates reads all YAML files under
// charts/sortie/templates/ and returns a set of all SORTIE_* names found.
func extractEnvVarsFromChartTemplates(t *testing.T) map[string]bool {
	t.Helper()

	chartDir := filepath.Join("..", "..", "charts", "sortie", "templates")
	entries, err := os.ReadDir(chartDir)
	if err != nil {
		t.Fatalf("failed to read chart templates dir %s: %v", chartDir, err)
	}

	re := regexp.MustCompile(`SORTIE_[A-Z0-9_]+`)
	vars := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(chartDir, entry.Name()))
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		for _, m := range re.FindAllString(string(data), -1) {
			vars[m] = true
		}
	}

	return vars
}
