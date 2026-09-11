package config

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Storage configures Google Cloud Storage for videos. An empty Bucket disables
// video routes. Credentials come from Application Default Credentials.
type Storage struct {
	Bucket string
	// SignerEmail is the service account that signs URLs through IAM when the
	// credentials themselves cannot sign, for example with workload identity.
	SignerEmail   string
	MaxVideoBytes int64
	UploadURLTTL  time.Duration
	ReadURLTTL    time.Duration
}

func (s Storage) Enabled() bool { return s.Bucket != "" }

var bucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$`)

// LoadStorage reads storage settings. Errors never echo configured values.
func LoadStorage(getenv func(string) string) (Storage, error) {
	s := Storage{Bucket: getenv("GCS_BUCKET"), SignerEmail: getenv("GCS_SIGNER_EMAIL"), MaxVideoBytes: 20 << 30}
	if s.Bucket != "" && !bucketName.MatchString(s.Bucket) {
		return Storage{}, errors.New("GCS_BUCKET must be a valid bucket name of 3 to 63 lowercase characters")
	}
	if s.SignerEmail != "" && !strings.Contains(s.SignerEmail, "@") {
		return Storage{}, errors.New("GCS_SIGNER_EMAIL must be a service account email")
	}
	if raw := getenv("VIDEO_MAX_BYTES"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1<<20 || n > 5<<40 {
			return Storage{}, errors.New("VIDEO_MAX_BYTES must be an integer from 1 MiB to 5 TiB")
		}
		s.MaxVideoBytes = n
	}
	var err error
	if s.UploadURLTTL, err = duration(getenv, "UPLOAD_URL_TTL", time.Hour); err != nil {
		return Storage{}, err
	}
	if s.UploadURLTTL < time.Minute || s.UploadURLTTL > 7*24*time.Hour {
		return Storage{}, errors.New("UPLOAD_URL_TTL must be between 1m and 168h")
	}
	if s.ReadURLTTL, err = duration(getenv, "READ_URL_TTL", 15*time.Minute); err != nil {
		return Storage{}, err
	}
	if s.ReadURLTTL < time.Minute || s.ReadURLTTL > 12*time.Hour {
		return Storage{}, errors.New("READ_URL_TTL must be between 1m and 12h")
	}
	return s, nil
}
