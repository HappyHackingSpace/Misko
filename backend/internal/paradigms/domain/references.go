package domain

// references are encyclopedia articles that introduce each paradigm, per
// language. They are reading material, not part of a version's contract, so
// they live outside the published manifests and may change at any time. A
// language without its own article falls back to English (Text.In).
var references = map[string]Text{
	"OPEN_FIELD":   {EN: "https://en.wikipedia.org/wiki/Open_field_(animal_test)"},
	"EPM":          {EN: "https://en.wikipedia.org/wiki/Elevated_plus_maze"},
	"Y_MAZE":       {EN: "https://en.wikipedia.org/wiki/Spontaneous_alternation", TR: "https://tr.wikipedia.org/wiki/Kendili%C4%9Finden_de%C4%9Fi%C5%9Fim"},
	"BARNES_MAZE":  {EN: "https://en.wikipedia.org/wiki/Barnes_maze"},
	"MWM":          {EN: "https://en.wikipedia.org/wiki/Morris_water_navigation_task"},
	"LIGHT_DARK":   {EN: "https://en.wikipedia.org/wiki/Light-dark_box_test"},
	"ROTAROD":      {EN: "https://en.wikipedia.org/wiki/Rotarod_performance_test"},
	"NOVEL_OBJECT": {EN: "https://en.wikipedia.org/wiki/Animal_cognition#Novel_object_recognition_test"},
}

// Reference is the article introducing a paradigm; empty when none exists.
func Reference(key string) Text { return references[key] }
