package config

import (
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestStorageIsOptionalAndValidated(t *testing.T) {
	s, err := LoadStorage(env(nil))
	if err != nil || s.Enabled() || s.MaxVideoBytes != 20<<30 || s.UploadURLTTL != time.Hour || s.ReadURLTTL != 15*time.Minute {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	s, err = LoadStorage(env(map[string]string{
		"GCS_BUCKET": "misko-lab-videos", "GCS_SIGNER_EMAIL": "api@misko-lab.iam.gserviceaccount.com",
		"VIDEO_MAX_BYTES": "1073741824", "UPLOAD_URL_TTL": "30m", "READ_URL_TTL": "5m",
	}))
	if err != nil || !s.Enabled() || s.Bucket != "misko-lab-videos" || s.SignerEmail == "" || s.MaxVideoBytes != 1<<30 || s.UploadURLTTL != 30*time.Minute || s.ReadURLTTL != 5*time.Minute {
		t.Fatalf("configured: %+v %v", s, err)
	}
	for name, values := range map[string]map[string]string{
		"uppercase bucket":     {"GCS_BUCKET": "Misko"},
		"bucket too short":     {"GCS_BUCKET": "ab"},
		"bucket with slash":    {"GCS_BUCKET": "misko/videos"},
		"signer without at":    {"GCS_BUCKET": "misko-videos", "GCS_SIGNER_EMAIL": "service-account"},
		"tiny maximum":         {"VIDEO_MAX_BYTES": "1024"},
		"maximum not a number": {"VIDEO_MAX_BYTES": "20GB"},
		"upload URL too long":  {"UPLOAD_URL_TTL": "169h"},
		"read URL too short":   {"READ_URL_TTL": "10s"},
		"read URL too long":    {"READ_URL_TTL": "13h"},
	} {
		_, err := LoadStorage(env(values))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		for _, v := range values {
			if strings.Contains(err.Error(), v) && len(v) > 3 {
				t.Errorf("%s: error echoes the value: %v", name, err)
			}
		}
	}
}
