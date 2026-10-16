package domain

// references are encyclopedia articles that introduce each paradigm, per
// language. They are reading material, not part of a version's contract, so
// they live outside the published manifests and may change at any time. A
// language without its own article falls back to English (Text.In).
var references = map[string]Text{
	"MWM":          {EN: "https://en.wikipedia.org/wiki/Morris_water_navigation_task"},
	"NOVEL_OBJECT": {EN: "https://en.wikipedia.org/wiki/Animal_cognition#Novel_object_recognition_test"},
}

// Reference is the article introducing a paradigm; empty when none exists.
func Reference(key string) Text { return references[key] }
