// Package domain holds the shapes the dashboard summary is made of. It has no
// rules of its own; every number comes from the experiments, tests, analysis
// and calibration domains it reads.
package domain

import "time"

type ExperimentCounts struct {
	Draft, Running, Completed int
}

type StatusCounts struct {
	Planned, InProgress, Completed, Cancelled int
}

// ResultCounts are the animals' analysis results by review status.
type ResultCounts struct {
	Pending, Accepted, Rejected int
}

// Activity is one published analysis result of one animal, newest first.
type Activity struct {
	TestID, TestSubjectID, SubjectCode, ExperimentCode, ParadigmKey string
	ParadigmVersion                                                 int
	FinishedAt                                                      time.Time
}

// UpcomingTest is one planned session, soonest first, with the animals in it.
type UpcomingTest struct {
	ID, ExperimentCode, TimepointName, ParadigmKey string
	ParadigmVersion                                int
	SubjectCodes                                   []string
	ScheduledAt                                    time.Time
}

type Summary struct {
	Experiments                                     ExperimentCounts
	Tests                                           StatusCounts
	AnalysisQueued, AnalysisRunning, AnalysisFailed int
	Results                                         ResultCounts
	CalibrationWaiting                              int
	RecentActivity                                  []Activity
	UpcomingTests                                   []UpcomingTest
}
