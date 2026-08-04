package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvolutionConfigTOMLAndJSONC(t *testing.T) {
	d := t.TempDir()
	toml := filepath.Join(d, "a.toml")
	if err := os.WriteFile(toml, []byte("[evolution]\nenabled = true\nmode = \"suggest\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(toml)
	if err != nil || !c.Evolution.Enabled || c.Evolution.Mode != "suggest" {
		t.Fatalf("toml: %+v %v", c, err)
	}
	jsonc := filepath.Join(d, "a.jsonc")
	if err := os.WriteFile(jsonc, []byte("{\"evolution\": {\"enabled\": true, \"mode\": \"suggest\"}}"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = Load(jsonc)
	if err != nil || !c.Evolution.Enabled || c.Evolution.Mode != "suggest" {
		t.Fatalf("jsonc: %+v %v", c, err)
	}
}
