package domain

// Rule codes name the start rules the database cannot express. They are
// stable, translated by the panel, and reported with the group or endpoint
// they concern.
const (
	RuleNoTimepoints            = "readiness.noTimepoints"
	RuleNoGroups                = "readiness.noGroups"
	RuleTargetSizeMissing       = "readiness.targetSizeMissing"
	RulePrimaryEndpointMissing  = "readiness.primaryEndpointMissing"
	RuleEndpointMetricMissing   = "readiness.endpointMetricUnavailable"
	RuleTreatmentItemMissing    = "readiness.treatmentItemMissing"
	RuleTreatmentItemNotAllowed = "readiness.treatmentItemNotAllowed"
	RuleInductionItemMissing    = "readiness.inductionItemMissing"
	RuleInductionItemNotAllowed = "readiness.inductionItemNotAllowed"
	RuleVehicleItemMissing      = "readiness.vehicleItemMissing"
	RuleDiseaseControlMissing   = "readiness.diseaseControlMissing"
)

// Problem is one unmet start rule. GroupID or EndpointID name what it is about
// when the rule concerns one group or endpoint.
type Problem struct {
	Rule       string
	GroupID    string
	EndpointID string
}

// Design is everything the start rules look at.
type Design struct {
	Groups     []Group
	Items      []RegimenItem
	Timepoints []Timepoint
	Endpoints  []Endpoint
	// Metrics holds, per paradigm key, the metric keys of the paradigm
	// versions the experiment's protocol steps use.
	Metrics map[string]map[string]bool
}

// MetricAvailable reports whether a protocol step of the study runs a paradigm
// version that computes the metric.
func (d Design) MetricAvailable(paradigm, metric string) bool { return d.Metrics[paradigm][metric] }

// Readiness lists every start rule the design breaks, in a stable order; an
// empty list means the study can start (DOMAIN_REDESIGN.md section 5).
func Readiness(d Design) []Problem {
	problems := []Problem{}
	if len(d.Timepoints) == 0 {
		problems = append(problems, Problem{Rule: RuleNoTimepoints})
	}
	if len(d.Groups) == 0 {
		problems = append(problems, Problem{Rule: RuleNoGroups})
	}
	primary := false
	for _, e := range d.Endpoints {
		primary = primary || e.Kind == Primary
	}
	if !primary {
		problems = append(problems, Problem{Rule: RulePrimaryEndpointMissing})
	}
	for _, e := range d.Endpoints {
		if !d.MetricAvailable(e.ParadigmKey, e.MetricKey) {
			problems = append(problems, Problem{Rule: RuleEndpointMetricMissing, EndpointID: e.ID})
		}
	}

	purposes := map[string]map[Purpose]int{}
	for _, item := range d.Items {
		if purposes[item.GroupID] == nil {
			purposes[item.GroupID] = map[Purpose]int{}
		}
		purposes[item.GroupID][item.Purpose]++
	}
	diseaseControls := map[string]bool{}
	for _, g := range d.Groups {
		if g.Role == DiseaseControl {
			diseaseControls[g.DiseaseModelID] = true
		}
	}
	for _, g := range d.Groups {
		add := func(rule string) { problems = append(problems, Problem{Rule: rule, GroupID: g.ID}) }
		if g.TargetSize == 0 {
			add(RuleTargetSizeMissing)
		}
		has := purposes[g.ID]
		switch g.Role {
		case Treatment, PositiveControl:
			if has[TreatmentPurpose] != 1 {
				add(RuleTreatmentItemMissing)
			}
			if has[Induction] == 0 {
				add(RuleInductionItemMissing)
			}
		case DiseaseControl:
			if has[Induction] == 0 {
				add(RuleInductionItemMissing)
			}
			if has[Vehicle] == 0 {
				add(RuleVehicleItemMissing)
			}
			if has[TreatmentPurpose] > 0 {
				add(RuleTreatmentItemNotAllowed)
			}
		case Sham:
			if has[Vehicle] == 0 {
				add(RuleVehicleItemMissing)
			}
			if has[Induction] > 0 {
				add(RuleInductionItemNotAllowed)
			}
			if has[TreatmentPurpose] > 0 {
				add(RuleTreatmentItemNotAllowed)
			}
		case HealthyControl:
			if has[Induction] > 0 {
				add(RuleInductionItemNotAllowed)
			}
			if has[TreatmentPurpose] > 0 {
				add(RuleTreatmentItemNotAllowed)
			}
		}
		if (g.Role == Sham || g.Role == Treatment || g.Role == PositiveControl) && !diseaseControls[g.DiseaseModelID] {
			add(RuleDiseaseControlMissing)
		}
	}
	return problems
}
