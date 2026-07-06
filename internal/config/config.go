// Package config defines the typed application configuration and loads it
// from a YAML file. All validation happens at load time so that the run loop
// can assume every field is well-formed.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

// Config is the root configuration.
type Config struct {
	Source   Source   `yaml:"source"`
	Rules    []Rule   `yaml:"rules"`
	Notifier Notifier `yaml:"notifier"`
	Store    Store    `yaml:"store"`
	Logging  Logging  `yaml:"logging"`
}

// SourceKind enumerates supported deal sources.
type SourceKind string

// Supported source kinds.
const (
	SourceKindUniqlo SourceKind = "uniqlo"
)

// Gender enumerates the top-level gender segments Uniqlo exposes.
// The string value is the human-readable name; each corresponds to a
// stable numeric gender ID used by the commerce API.
type Gender string

// Supported gender values.
const (
	GenderMen   Gender = "men"
	GenderWomen Gender = "women"
	GenderKids  Gender = "kids"
	GenderBaby  Gender = "baby"
)

// GenderID returns the numeric ID Uniqlo's API expects for this Gender.
// Returns 0 for an unknown value (validate() rejects those at load time).
func (g Gender) GenderID() int {
	switch g {
	case GenderWomen:
		return 37608
	case GenderMen:
		return 37609
	case GenderKids:
		return 37610
	case GenderBaby:
		return 37611
	default:
		return 0
	}
}

// Source configures where deals are fetched from.
type Source struct {
	Kind              SourceKind    `yaml:"kind"`
	BaseURL           string        `yaml:"baseURL"`
	Region            string        `yaml:"region"`
	Language          string        `yaml:"language"`
	Gender            Gender        `yaml:"gender"`
	SizeCodes         []string      `yaml:"sizeCodes"`
	Sort              int           `yaml:"sort"`
	ClientID          string        `yaml:"clientID"`
	ClientVersion     string        `yaml:"clientVersion"`
	RequestsPerSecond float64       `yaml:"requestsPerSecond"`
	Timeout           time.Duration `yaml:"timeout"`
	MaxRetries        int           `yaml:"maxRetries"`
	UserAgent         string        `yaml:"userAgent"`
}

// Rule is a filter. A deal matches a Rule iff every non-empty condition matches.
type Rule struct {
	Name               string          `yaml:"name"`
	NamePattern        string          `yaml:"namePattern"`
	Sizes              []string        `yaml:"sizes"`
	MaxPriceEUR        decimal.Decimal `yaml:"maxPriceEUR"`
	MinDiscountPercent int             `yaml:"minDiscountPercent"`

	// compiled holds the compiled NamePattern. Populated by validate().
	// Exposed via NameRegex() so callers do not need to recompile.
	compiled *regexp.Regexp
}

// NameRegex returns the compiled name pattern (may be nil if unset).
func (r Rule) NameRegex() *regexp.Regexp { return r.compiled }

// NotifierKind enumerates supported notifier backends.
type NotifierKind string

// Supported notifier kinds.
const (
	NotifierKindSMTP NotifierKind = "smtp"
)

// Notifier configures the outbound notification channel.
type Notifier struct {
	Kind NotifierKind `yaml:"kind"`
	SMTP SMTP         `yaml:"smtp"`
}

// SMTP configures the SMTP notifier.
type SMTP struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	StartTLS     bool          `yaml:"startTLS"`
	From         string        `yaml:"from"`
	To           []string      `yaml:"to"`
	Username     string        `yaml:"username"`
	PasswordFile string        `yaml:"passwordFile"`
	Timeout      time.Duration `yaml:"timeout"`
}

// StoreKind enumerates supported persistence backends.
type StoreKind string

// Supported store kinds.
const (
	StoreKindSQLite StoreKind = "sqlite"
)

// Store configures the persistence backend for dedup state.
type Store struct {
	Kind          StoreKind `yaml:"kind"`
	Path          string    `yaml:"path"`
	RetentionDays int       `yaml:"retentionDays"`
}

// Logging configures the logger.
type Logging struct {
	Level string `yaml:"level"`
}

// DefaultUserAgent is used when the operator does not override
// source.userAgent in config. Uniqlo's Akamai front-end responds with
// HTTP/2 INTERNAL_ERROR to obviously-bot-shaped UAs, so we ship a stable
// modern-Chrome UA string. Bumping the Chrome major version once or twice
// a year is enough; ownership sits with whoever bumps the Go module.
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// ErrInvalidConfig is returned when Config validation fails.
var ErrInvalidConfig = errors.New("invalid config")

// Load reads and validates a YAML file. The returned Config is guaranteed
// to have all required fields populated and any regex patterns compiled.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is operator-controlled
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(newBytesReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// applyDefaults fills in the sensible defaults so the YAML file only needs
// to specify what a user actually cares about.
func (c *Config) applyDefaults() {
	if c.Source.Timeout == 0 {
		c.Source.Timeout = 15 * time.Second
	}
	if c.Source.MaxRetries == 0 {
		c.Source.MaxRetries = 3
	}
	if c.Source.RequestsPerSecond == 0 {
		c.Source.RequestsPerSecond = 1
	}
	if c.Source.UserAgent == "" {
		c.Source.UserAgent = DefaultUserAgent
	}
	if c.Source.ClientVersion == "" {
		// Fallback x-fr-client-version used only when live discovery from
		// the Uniqlo storefront fails at startup. Under normal conditions
		// the client fetches the current window.__BUILD_VERSION__ at
		// runtime; this string is a floor. Bumping it manually is not
		// required but keeps the fallback fresh.
		c.Source.ClientVersion = "3.2509.1"
	}
	if c.Notifier.SMTP.Timeout == 0 {
		c.Notifier.SMTP.Timeout = 15 * time.Second
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Store.RetentionDays == 0 {
		c.Store.RetentionDays = 90
	}
}

// validate runs all cross-field checks and compiles regex patterns.
// Keeps cyclomatic complexity low by delegating each section to a helper.
func (c *Config) validate() error {
	for _, v := range []func() error{
		c.validateSource,
		c.validateRules,
		c.validateNotifier,
		c.validateStore,
	} {
		if err := v(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateSource() error {
	if c.Source.Kind != SourceKindUniqlo {
		return fmt.Errorf("%w: source.kind must be %q, got %q", ErrInvalidConfig, SourceKindUniqlo, c.Source.Kind)
	}
	if c.Source.BaseURL == "" {
		return fmt.Errorf("%w: source.baseURL is required", ErrInvalidConfig)
	}
	if c.Source.Region == "" || c.Source.Language == "" {
		return fmt.Errorf("%w: source.region and language are required", ErrInvalidConfig)
	}
	if c.Source.Gender.GenderID() == 0 {
		return fmt.Errorf("%w: source.gender must be one of men|women|kids|baby, got %q",
			ErrInvalidConfig, c.Source.Gender)
	}
	if c.Source.ClientID == "" {
		return fmt.Errorf("%w: source.clientID is required", ErrInvalidConfig)
	}
	return nil
}

func (c *Config) validateRules() error {
	if len(c.Rules) == 0 {
		return fmt.Errorf("%w: at least one rule is required", ErrInvalidConfig)
	}
	for i := range c.Rules {
		r := &c.Rules[i]
		if r.Name == "" {
			return fmt.Errorf("%w: rules[%d].name is required", ErrInvalidConfig, i)
		}
		if r.NamePattern != "" {
			re, err := regexp.Compile(r.NamePattern)
			if err != nil {
				return fmt.Errorf("%w: rules[%d].namePattern: %w", ErrInvalidConfig, i, err)
			}
			r.compiled = re
		}
		if r.MinDiscountPercent < 0 || r.MinDiscountPercent > 100 {
			return fmt.Errorf("%w: rules[%d].minDiscountPercent must be in [0,100]", ErrInvalidConfig, i)
		}
	}
	return nil
}

func (c *Config) validateNotifier() error {
	if c.Notifier.Kind != NotifierKindSMTP {
		return fmt.Errorf("%w: notifier.kind must be %q, got %q", ErrInvalidConfig, NotifierKindSMTP, c.Notifier.Kind)
	}
	s := c.Notifier.SMTP
	switch {
	case s.Host == "":
		return fmt.Errorf("%w: notifier.smtp.host is required", ErrInvalidConfig)
	case s.Port <= 0 || s.Port > 65535:
		return fmt.Errorf("%w: notifier.smtp.port must be in (0,65535]", ErrInvalidConfig)
	case s.From == "":
		return fmt.Errorf("%w: notifier.smtp.from is required", ErrInvalidConfig)
	case len(s.To) == 0:
		return fmt.Errorf("%w: notifier.smtp.to must have at least one recipient", ErrInvalidConfig)
	case s.PasswordFile == "":
		return fmt.Errorf("%w: notifier.smtp.passwordFile is required", ErrInvalidConfig)
	}
	return nil
}

func (c *Config) validateStore() error {
	if c.Store.Kind != StoreKindSQLite {
		return fmt.Errorf("%w: store.kind must be %q, got %q", ErrInvalidConfig, StoreKindSQLite, c.Store.Kind)
	}
	if c.Store.Path == "" {
		return fmt.Errorf("%w: store.path is required", ErrInvalidConfig)
	}
	return nil
}
