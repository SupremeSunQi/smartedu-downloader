package browserauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	smartEduOrigin      = "https://basic.smartedu.cn"
	smartEduTokenKey    = "ND_UC_AUTH-e5649925-441d-4a53-b525-51a2f1c4e0a8&ncet-xedu&token"
	maxStoredValueBytes = 16 * 1024
	defaultMaxFiles     = 4096
	defaultMaxBytes     = 128 << 20
)

var (
	ErrExpired             = errors.New("browser session is expired")
	ErrIncompatibleStorage = errors.New("browser session storage is incompatible")
	ErrStorage             = errors.New("browser session storage is unavailable")
	profileNamePattern     = regexp.MustCompile(`^Profile [0-9]+$`)
)

type Candidate struct {
	Browser string
	Profile string
	Token   string
}

type Options struct {
	ChromeRoot       string
	EdgeRoot         string
	Now              func() time.Time
	MaxSnapshotFiles int
	MaxSnapshotBytes int64
}

type Reader struct {
	options Options
}

func NewReader(options Options) *Reader {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.MaxSnapshotFiles <= 0 {
		options.MaxSnapshotFiles = defaultMaxFiles
	}
	if options.MaxSnapshotBytes <= 0 {
		options.MaxSnapshotBytes = defaultMaxBytes
	}
	return &Reader{options: options}
}

func DefaultWindowsOptions(localAppData string) Options {
	return Options{
		ChromeRoot: filepath.Join(localAppData, "Google", "Chrome", "User Data"),
		EdgeRoot:   filepath.Join(localAppData, "Microsoft", "Edge", "User Data"),
	}
}

func (reader *Reader) Candidates(ctx context.Context) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type browserRoot struct {
		name string
		path string
	}
	roots := []browserRoot{{name: "Chrome", path: reader.options.ChromeRoot}, {name: "Edge", path: reader.options.EdgeRoot}}
	candidates := make([]Candidate, 0, 2)
	seenTokens := make(map[string]struct{})
	var profileErrors []error
	foundExpired := false
	for _, root := range roots {
		profiles, err := profileDirectories(root.path)
		if err != nil {
			profileErrors = append(profileErrors, fmt.Errorf("%s profiles: %w", root.name, err))
			continue
		}
		for _, profile := range profiles {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			tokens, err := reader.readProfile(ctx, filepath.Join(root.path, profile, "Local Storage", "leveldb"))
			if errors.Is(err, ErrExpired) {
				foundExpired = true
				continue
			}
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				profileErrors = append(profileErrors, fmt.Errorf("%s profile %s: %w", root.name, profile, err))
				continue
			}
			for _, token := range tokens {
				if _, duplicate := seenTokens[token]; duplicate {
					continue
				}
				seenTokens[token] = struct{}{}
				candidates = append(candidates, Candidate{Browser: root.name, Profile: profile, Token: token})
			}
		}
	}
	if len(candidates) == 0 && len(profileErrors) > 0 {
		return nil, errors.Join(append([]error{ErrStorage}, profileErrors...)...)
	}
	if len(candidates) == 0 && foundExpired {
		return nil, ErrExpired
	}
	return candidates, nil
}

func profileDirectories(root string) ([]string, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == "Default" || profileNamePattern.MatchString(entry.Name()) {
			profiles = append(profiles, entry.Name())
		}
	}
	sort.Slice(profiles, func(left, right int) bool {
		if profiles[left] == "Default" {
			return true
		}
		if profiles[right] == "Default" {
			return false
		}
		return profiles[left] < profiles[right]
	})
	return profiles, nil
}
