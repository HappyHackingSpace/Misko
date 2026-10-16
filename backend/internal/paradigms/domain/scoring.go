package domain

// ObserverScored reports whether a person scores the paradigm on the video
// instead of a worker tracking the animal: the engine consumes observed
// events and the paradigm has no zones a trajectory could be evaluated in.
// No worker ever analyses such a paradigm.
func (p Paradigm) ObserverScored() bool {
	return len(p.InputEvents) > 0 && len(p.Zones) == 0
}
