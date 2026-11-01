package trialshttp

import (
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/trials/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/domain"
)

type trialJSON struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	ParadigmKey           string    `json:"paradigmKey"`
	ParadigmVersion       int       `json:"paradigmVersion"`
	EnvironmentRevisionID string    `json:"environmentRevisionId"`
	VideoID               *string   `json:"videoId"`
	ClipStartUs           int64     `json:"clipStartUs"`
	ClipEndUs             *int64    `json:"clipEndUs"`
	Notes                 *string   `json:"notes"`
	CreatedBy             string    `json:"createdBy"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

func toTrial(t domain.Trial) trialJSON {
	return trialJSON{ID: t.ID, Name: t.Name, ParadigmKey: t.ParadigmKey, ParadigmVersion: t.ParadigmVersion, EnvironmentRevisionID: t.EnvironmentRevisionID,
		VideoID: nullable(t.VideoAssetID), ClipStartUs: t.Clip.StartUs, ClipEndUs: t.Clip.EndUs, Notes: nullable(t.Notes), CreatedBy: t.CreatedBy,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

type arenaJSON struct {
	EnvironmentRevisionID string             `json:"environmentRevisionId"`
	EnvironmentID         string             `json:"environmentId"`
	EnvironmentName       string             `json:"environmentName"`
	Revision              int                `json:"revision"`
	Parameters            map[string]float64 `json:"parameters,omitempty"`
}

type jobJSON struct {
	ID              string             `json:"id"`
	Status          string             `json:"status"`
	ParadigmKey     string             `json:"paradigmKey"`
	ParadigmVersion int                `json:"paradigmVersion"`
	Parameters      map[string]float64 `json:"parameters"`
	SourceVideoID   string             `json:"sourceVideoId"`
	CalibrationID   *string            `json:"calibrationId"`
	Attempt         int                `json:"attempt"`
	MaxAttempts     int                `json:"maxAttempts"`
	ModelVersion    *string            `json:"modelVersion"`
	FailureReason   *string            `json:"failureReason"`
	CreatedAt       time.Time          `json:"createdAt"`
	FinishedAt      *time.Time         `json:"finishedAt"`
}

func toJob(j domain.Job) jobJSON {
	params := j.Parameters
	if params == nil {
		params = map[string]float64{}
	}
	return jobJSON{ID: j.ID, Status: j.Status, ParadigmKey: j.ParadigmKey, ParadigmVersion: j.ParadigmVersion, Parameters: params,
		SourceVideoID: j.SourceAssetID, CalibrationID: nullable(j.CalibrationID), Attempt: j.Attempt, MaxAttempts: j.MaxAttempts,
		ModelVersion: nullable(j.ModelVersion), FailureReason: nullable(j.FailureReason), CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt}
}

// summaryJSON is a trial in the list: its arena and its newest analysis.
type summaryJSON struct {
	trialJSON
	Arena  arenaJSON `json:"arena"`
	Latest *jobJSON  `json:"latestJob"`
}

func toSummary(s application.Summary) summaryJSON {
	out := summaryJSON{trialJSON: toTrial(s.Trial), Arena: arenaJSON{EnvironmentRevisionID: s.Trial.EnvironmentRevisionID, EnvironmentID: s.EnvironmentID,
		EnvironmentName: s.EnvironmentName, Revision: s.RevisionNumber}}
	if s.Latest != nil {
		j := toJob(*s.Latest)
		out.Latest = &j
	}
	return out
}

type videoJSON struct {
	ID              string     `json:"id"`
	FileName        *string    `json:"fileName"`
	ContentType     string     `json:"contentType"`
	SizeBytes       int64      `json:"sizeBytes"`
	Status          string     `json:"status"`
	RejectionReason *string    `json:"rejectionReason"`
	CreatedAt       time.Time  `json:"createdAt"`
	VerifiedAt      *time.Time `json:"verifiedAt"`
}

func toVideo(v domain.Video) videoJSON {
	return videoJSON{ID: v.ID, FileName: nullable(v.FileName), ContentType: v.ContentType, SizeBytes: v.SizeBytes, Status: v.Status,
		RejectionReason: nullable(v.RejectionReason), CreatedAt: v.CreatedAt, VerifiedAt: v.VerifiedAt}
}

type uploadJSON struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

type metricJSON struct {
	Key           string   `json:"key"`
	Unit          string   `json:"unit"`
	Value         *float64 `json:"value"`
	MissingReason *string  `json:"missingReason"`
}

type qcJSON struct {
	Samples        int     `json:"samples"`
	Tracked        int     `json:"tracked"`
	Inferred       int     `json:"inferred"`
	LostRatio      float64 `json:"lostRatio"`
	InferredRatio  float64 `json:"inferredRatio"`
	MeanConfidence float64 `json:"meanConfidence"`
	Passed         bool    `json:"passed"`
}

// analysisJSON is one analysis of a trial: why and with what it ran, the
// job's state, its metrics and QC.
type analysisJSON struct {
	JobID         string             `json:"jobId"`
	Trigger       string             `json:"trigger"`
	VideoID       string             `json:"videoId"`
	CalibrationID *string            `json:"calibrationId"`
	Overrides     map[string]float64 `json:"overrides"`
	CreatedBy     *string            `json:"createdBy"`
	CreatedAt     time.Time          `json:"createdAt"`
	Job           jobJSON            `json:"job"`
	Metrics       []metricJSON       `json:"metrics"`
	QC            *qcJSON            `json:"qc"`
}

func toAnalysis(a domain.Analysis) analysisJSON {
	overrides := a.Overrides
	if overrides == nil {
		overrides = map[string]float64{}
	}
	out := analysisJSON{JobID: a.JobID, Trigger: string(a.Trigger), VideoID: a.VideoAssetID, CalibrationID: nullable(a.CalibrationID),
		Overrides: overrides, CreatedBy: nullable(a.CreatedBy), CreatedAt: a.CreatedAt, Job: toJob(a.Job), Metrics: []metricJSON{}}
	for _, m := range a.Metrics {
		out.Metrics = append(out.Metrics, metricJSON{m.Key, m.Unit, m.Value, nullable(m.MissingReason)})
	}
	if q := a.QC; q != nil {
		out.QC = &qcJSON{q.Samples, q.Tracked, q.Inferred, q.LostRatio, q.InferredRatio, q.MeanConfidence, q.Passed}
	}
	return out
}

// detailJSON is the trial page: the trial, its arena, its video, the
// calibration it would be analysed with now and every analysis, newest first.
type detailJSON struct {
	trialJSON
	Arena         arenaJSON      `json:"arena"`
	Video         *videoJSON     `json:"video"`
	CalibrationID *string        `json:"calibrationId"`
	Analyses      []analysisJSON `json:"analyses"`
}

func toDetail(d application.Detail) detailJSON {
	out := detailJSON{trialJSON: toTrial(d.Trial), Arena: arenaJSON{EnvironmentRevisionID: d.Revision.ID, EnvironmentID: d.Revision.EnvironmentID,
		EnvironmentName: d.Revision.EnvironmentName, Revision: d.Revision.Number, Parameters: d.Revision.Parameters},
		CalibrationID: nullable(d.CalibrationID), Analyses: mapSlice(d.Analyses, toAnalysis)}
	if d.Video != nil {
		v := toVideo(*d.Video)
		out.Video = &v
	}
	return out
}

func mapSlice[T, U any](items []T, convert func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
