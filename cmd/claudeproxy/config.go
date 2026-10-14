package claudeproxy

import (
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"

	"github.com/JLugagne/claudeproxy/internal/pkg/config"
	proxyconfig "github.com/JLugagne/claudeproxy/internal/proxy/config"
)

type fileConfig struct {
	Server config.Server      `yaml:"server"`
	Proxy  proxyconfig.Config `yaml:"proxy"`
}

func loadConfig(path string) (fileConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return fileConfig{}, errors.Join(errors.New("open configuration"), err)
	}
	defer f.Close()

	var cfg fileConfig
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return fileConfig{}, errors.Join(fmt.Errorf("parse %s", path), err)
	}
	cfg.Server = cfg.Server.WithDefaults()
	if cfg.Proxy, err = cfg.Proxy.Resolve(); err != nil {
		return fileConfig{}, errors.Join(fmt.Errorf("resolve %s", path), err)
	}
	return cfg, nil
}
