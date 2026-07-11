package syncer

import (
	"fmt"

	"github.com/rclone/rclone/fs/filter"
)

// BuildFilterRules compiles exclude and include patterns into an ordered list
// of rclone filter rules.
//
// Rules are matched first-match-wins, so excludes are emitted before includes
// and therefore take precedence over them. When at least one include is given,
// a catch-all is appended so that anything not included is skipped.
//
// Rules are returned for rclone's FilterRule rather than its IncludeRule and
// ExcludeRule: rclone parses those two in a fixed order regardless of intent,
// which makes their precedence indeterminate when both are set.
func BuildFilterRules(exclude, include []string) ([]string, error) {
	rules := make([]string, 0, len(exclude)+len(include)+1)

	for _, pattern := range exclude {
		if err := validatePattern(pattern); err != nil {
			return nil, err
		}
		rules = append(rules, "- "+pattern)
	}

	for _, pattern := range include {
		if err := validatePattern(pattern); err != nil {
			return nil, err
		}
		rules = append(rules, "+ "+pattern)
	}

	// rclone only adds an implicit catch-all for IncludeRule, which we don't
	// use, so emit it ourselves.
	if len(include) > 0 {
		rules = append(rules, "- /**")
	}

	return rules, nil
}

func validatePattern(pattern string) error {
	if _, err := filter.GlobPathToRegexp(pattern, false); err != nil {
		return fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	return nil
}
