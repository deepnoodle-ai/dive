package sandbox

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func validateNetworkConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	if len(cfg.Network.AllowedDomains) > 0 {
		if !cfg.AllowNetwork {
			return fmt.Errorf("allowed_domains requires allow_network=true")
		}
	}
	return nil
}

func buildProxyEnv(cfg *Config) map[string]string {
	envs := map[string]string{}
	if cfg == nil {
		return envs
	}
	if cfg.Network.HTTPProxy != "" {
		envs["HTTP_PROXY"] = cfg.Network.HTTPProxy
		envs["http_proxy"] = cfg.Network.HTTPProxy
	}
	if cfg.Network.HTTPSProxy != "" {
		envs["HTTPS_PROXY"] = cfg.Network.HTTPSProxy
		envs["https_proxy"] = cfg.Network.HTTPSProxy
	}
	if len(cfg.Network.NoProxy) > 0 {
		noProxy := strings.Join(cfg.Network.NoProxy, ",")
		envs["NO_PROXY"] = noProxy
		envs["no_proxy"] = noProxy
	}
	if len(cfg.Network.AllowedDomains) > 0 {
		envs["DIVE_ALLOWED_DOMAINS"] = strings.Join(cfg.Network.AllowedDomains, ",")
	}
	return envs
}

func mergeEnv(base []string, additions map[string]string) []string {
	env := map[string]string{}
	if base == nil {
		base = os.Environ()
	}
	for _, kv := range base {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	for k, v := range additions {
		env[k] = v
	}
	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(env))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%s", k, env[k]))
	}
	return out
}

// BuildCommandEnv merges proxy and custom env vars with the base environment.
func BuildCommandEnv(base []string, cfg *Config) []string {
	envs := buildProxyEnv(cfg)
	if cfg != nil {
		for k, v := range cfg.Environment {
			envs[k] = v
		}
	}
	return mergeEnv(base, envs)
}
