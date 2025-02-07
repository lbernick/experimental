/*
Copyright 2021 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	cm "knative.dev/pkg/configmap"
)

// Config holds the collection of configurations that we attach to contexts.
// Configmap named with "config-trusted-resources" where cosign pub key path and
// KMS pub key path can be configured
type Config struct {
	// CosignKey defines the name of the key in configmap data
	CosignKey string
	// KmsKey defines the name of the key in configmap data
	KMSKey string
	// SkipValidation defines the flag to skip task run validation
	SkipValidation bool
}

const (
	// CosignPubKey is the name of the key in configmap data
	CosignPubKey = "cosign-pubkey-path"
	// SecretPath is the default path of cosign public key
	DefaultSecretPath = "/etc/signing-secrets/cosign.pub"
	// CosignPubKey is the name of the key in configmap data
	KMSPubKey = "kms-pubkey-path"
	// TrustedTaskConfig is the name of the trusted resources configmap
	TrustedTaskConfig = "config-trusted-resources"
	// SkipTaskRunValidation is the flag to skip task run validation
	PassTaskRunWhenFailVerification = "pass-taskrun-when-fail-verification"
)

func defaultConfig() *Config {
	return &Config{
		CosignKey: DefaultSecretPath,
	}
}

// NewConfigFromMap creates a Config from the supplied map
func NewConfigFromMap(data map[string]string) (*Config, error) {
	cfg := defaultConfig()
	if err := cm.Parse(data,
		cm.AsString(CosignPubKey, &cfg.CosignKey),
		cm.AsString(KMSPubKey, &cfg.KMSKey),
		cm.AsBool(PassTaskRunWhenFailVerification, &cfg.SkipValidation),
	); err != nil {
		return nil, fmt.Errorf("failed to parse data: %w", err)
	}
	return cfg, nil
}

// NewConfigFromConfigMap creates a Config from the supplied ConfigMap
func NewConfigFromConfigMap(configMap *corev1.ConfigMap) (*Config, error) {
	return NewConfigFromMap(configMap.Data)
}
