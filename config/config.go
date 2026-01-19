package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AlertDays          int         `yaml:"alertDays"`
	Telegram           Telegram    `yaml:"telegram"`
	CloudflareAccounts []CF        `yaml:"cloudflareAccounts"`
	Registrars         []Registrar `yaml:"registrars"`
	DomainFiles        []string    `yaml:"domainFiles"`

	AWSTargets map[string]AWSTarget `yaml:"awsTargets"`
}

type Telegram struct {
	BotToken string `yaml:"botToken"`
	ChatID   int64  `yaml:"chatID"`
}

type CF struct {
	Label     string `yaml:"label"`
	Email     string `yaml:"email"`
	APIToken  string `yaml:"apiToken"`
	AccountID string `yaml:"accountID"`
}
type Registrar struct {
	Label     string           `yaml:"label"`
	Type      string           `yaml:"type"`
	Namecheap *NamecheapConfig `yaml:"namecheap"`
	GoDaddy   *GoDaddyConfig   `yaml:"godaddy"`
}

type NamecheapConfig struct {
	User     string `yaml:"user"`
	APIKey   string `yaml:"apiKey"`
	ClientIP string `yaml:"clientIP"`
}

type GoDaddyConfig struct {
	APIKey    string `yaml:"apiKey"`
	APISecret string `yaml:"apiSecret"`
}
type AWSCreds struct {
	AccessKeyID     string `yaml:"accessKeyId"`
	SecretAccessKey string `yaml:"secretAccessKey"`
	SessionToken    string `yaml:"sessionToken"`
}

type AWSTarget struct {
	Region string   `yaml:"region"`
	Creds  AWSCreds `yaml:"creds"`
}

var Cfg Config

func Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := yaml.Unmarshal(data, &Cfg); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}
	return nil
}
