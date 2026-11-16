package nginxproxy

import (
	"log/slog"
	"sort"
	"strings"
)

// Recognized nginx-proxy hostname label keys.
//
// virtualHostLabel is the literal env-var name used as a Docker label key
// (jwilder/nginx-proxy convention, ported verbatim into the label namespace).
// canonicalLabel is the reverse-DNS canonical form that some users prefer.
const (
	virtualHostLabel = "VIRTUAL_HOST"
	canonicalLabel   = "com.nginx-proxy.virtual_host"
)

// Extraction represents a single hostname discovered from a label.
type Extraction struct {
	// Hostname is the extracted hostname (lowercased, trimmed).
	Hostname string
	// Router is the label key the hostname was parsed from.
	Router string
}

// Parser converts nginx-proxy style labels into hostname extractions.
type Parser struct {
	logger *slog.Logger
}

// ParserOption configures a Parser.
type ParserOption func(*Parser)

// WithParserLogger sets a custom logger on the parser.
func WithParserLogger(logger *slog.Logger) ParserOption {
	return func(p *Parser) {
		p.logger = logger
	}
}

// NewParser creates a parser with the given options.
func NewParser(opts ...ParserOption) *Parser {
	p := &Parser{logger: slog.Default()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ExtractHostnames returns all hostnames declared on recognized nginx-proxy
// labels. Values may contain one or more hostnames separated by commas or
// whitespace. Results are deterministic (sorted by label key, then by
// hostname order in the value) with duplicates removed.
func (p *Parser) ExtractHostnames(labels map[string]string) []Extraction {
	if len(labels) == 0 {
		return nil
	}

	keys := make([]string, 0, 2)
	for _, key := range []string{virtualHostLabel, canonicalLabel} {
		if _, ok := labels[key]; ok {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)

	seen := make(map[string]struct{})
	extractions := make([]Extraction, 0)

	for _, key := range keys {
		for _, host := range splitHostnames(labels[key]) {
			if _, dup := seen[host]; dup {
				continue
			}
			seen[host] = struct{}{}
			extractions = append(extractions, Extraction{
				Hostname: host,
				Router:   key,
			})
		}
	}

	return extractions
}

// splitHostnames splits a VIRTUAL_HOST value into normalized hostnames.
//
// jwilder/nginx-proxy treats VIRTUAL_HOST as a comma-separated list. We also
// accept whitespace separation to be forgiving of label-shell-escaping
// accidents, matching the behavior of the caddy source.
func splitHostnames(value string) []string {
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		h := strings.ToLower(strings.TrimSpace(f))
		if h == "" {
			continue
		}
		out = append(out, h)
	}
	return out
}
