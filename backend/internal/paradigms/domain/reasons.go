package domain

// MissingReasons lists every reason a metric may be reported missing.
func MissingReasons() []string {
	return []string{
		ReasonNoValidIntervals, ReasonNotObserved, ReasonZeroDenominator,
		ReasonIncompleteRecording, ReasonBelowCriterion,
	}
}
