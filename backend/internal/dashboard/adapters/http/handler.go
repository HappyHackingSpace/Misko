// Package dashboardhttp exposes the one read the dashboard page needs.
package dashboardhttp

import (
	"log/slog"
	"net/http"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/dashboard/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/dashboard/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpjson"
)

var failures = []httpjson.Failure{
	{Err: access.ErrUnauthenticated, Status: http.StatusUnauthorized, Code: "auth.unauthenticated"},
	{Err: access.ErrForbidden, Status: http.StatusForbidden, Code: "auth.forbidden"},
}

type handler struct {
	service      *application.Service
	authenticate func(*http.Request) (access.Actor, error)
	logger       *slog.Logger
}

func Register(mux *http.ServeMux, service *application.Service, authenticate func(*http.Request) (access.Actor, error), logger *slog.Logger) {
	h := handler{service: service, authenticate: authenticate, logger: logger}
	mux.HandleFunc("GET /api/dashboard/summary", h.with(h.summary))
}

func (h handler) with(next func(http.ResponseWriter, *http.Request, access.Actor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, err := h.authenticate(r)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		next(w, r, actor)
	}
}

func (h handler) summary(w http.ResponseWriter, r *http.Request, actor access.Actor) {
	summary, err := h.service.Summary(r.Context(), actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpjson.Write(w, http.StatusOK, summaryJSON(summary))
}

type statusCountsJSON struct {
	Planned    int `json:"planned"`
	InProgress int `json:"inProgress"`
	Completed  int `json:"completed"`
	Cancelled  int `json:"cancelled"`
}

type activityJSON struct {
	TestID          string    `json:"testId"`
	SubjectCode     string    `json:"subjectCode"`
	ExperimentCode  string    `json:"experimentCode"`
	ParadigmKey     string    `json:"paradigmKey"`
	ParadigmVersion int       `json:"paradigmVersion"`
	FinishedAt      time.Time `json:"finishedAt"`
}

type upcomingTestJSON struct {
	ID              string    `json:"id"`
	SubjectCode     string    `json:"subjectCode"`
	ExperimentCode  string    `json:"experimentCode"`
	ParadigmKey     string    `json:"paradigmKey"`
	ParadigmVersion int       `json:"paradigmVersion"`
	ScheduledAt     time.Time `json:"scheduledAt"`
}

type summaryJSONBody struct {
	Tests              statusCountsJSON   `json:"tests"`
	AnalysisQueued     int                `json:"analysisQueued"`
	AnalysisRunning    int                `json:"analysisRunning"`
	AnalysisFailed     int                `json:"analysisFailed"`
	CalibrationWaiting int                `json:"calibrationWaiting"`
	RecentActivity     []activityJSON     `json:"recentActivity"`
	UpcomingTests      []upcomingTestJSON `json:"upcomingTests"`
}

func summaryJSON(s domain.Summary) summaryJSONBody {
	activity := make([]activityJSON, 0, len(s.RecentActivity))
	for _, a := range s.RecentActivity {
		activity = append(activity, activityJSON{
			TestID: a.TestID, SubjectCode: a.SubjectCode, ExperimentCode: a.ExperimentCode,
			ParadigmKey: a.ParadigmKey, ParadigmVersion: a.ParadigmVersion,
			FinishedAt: a.FinishedAt,
		})
	}
	upcoming := make([]upcomingTestJSON, 0, len(s.UpcomingTests))
	for _, u := range s.UpcomingTests {
		upcoming = append(upcoming, upcomingTestJSON{
			ID: u.ID, SubjectCode: u.SubjectCode, ExperimentCode: u.ExperimentCode,
			ParadigmKey: u.ParadigmKey, ParadigmVersion: u.ParadigmVersion,
			ScheduledAt: u.ScheduledAt,
		})
	}
	return summaryJSONBody{
		Tests: statusCountsJSON{
			Planned: s.Tests.Planned, InProgress: s.Tests.InProgress,
			Completed: s.Tests.Completed, Cancelled: s.Tests.Cancelled,
		},
		AnalysisQueued: s.AnalysisQueued, AnalysisRunning: s.AnalysisRunning, AnalysisFailed: s.AnalysisFailed,
		CalibrationWaiting: s.CalibrationWaiting,
		RecentActivity:     activity,
		UpcomingTests:      upcoming,
	}
}

func (h handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpjson.Fail(w, r, h.logger, err, failures)
}
