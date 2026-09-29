// Package domain holds the shapes the dashboard summary is made of. It has no
// rules of its own; every number comes from the tests, analysis and
// calibration domains it reads.
package domain

import "time"

type StatusCounts struct {
	Planned, InProgress, Completed, Cancelled int
}

// Activity is one published analysis result, newest first.
type Activity struct {
	TestID, SubjectCode, ExperimentCode, ParadigmKey string
	ParadigmVersion                                  int
	FinishedAt                                       time.Time
}

// UpcomingTest is one planned test, soonest first.
type UpcomingTest struct {
	ID, SubjectCode, ExperimentCode, ParadigmKey string
	ParadigmVersion                              int
	ScheduledAt                                  time.Time
}

type Summary struct {
	Tests                                           StatusCounts
	AnalysisQueued, AnalysisRunning, AnalysisFailed int
	CalibrationWaiting                              int
	RecentActivity                                  []Activity
	UpcomingTests                                   []UpcomingTest
}
