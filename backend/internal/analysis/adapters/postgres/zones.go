package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

// zoneJSON is one zone shape of a calibration snapshot, stored the way the
// calibration context stores its zones: a circle as {"x", "y", "radius"}, a
// polygon as an array of {"x", "y"} vertices.
type zoneJSON struct {
	Key         string          `json:"key"`
	Kind        string          `json:"kind"`
	Pixels      json.RawMessage `json:"pixels"`
	Centimeters json.RawMessage `json:"centimeters"`
}

type vertexJSON struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type circleJSON struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Radius float64 `json:"radius"`
}

func encodeZones(zones []domain.Zone) ([]byte, error) {
	out := make([]zoneJSON, 0, len(zones))
	for _, z := range zones {
		kind, pixels, err := encodeShape(z.Pixels)
		if err != nil {
			return nil, err
		}
		_, centimeters, err := encodeShape(z.Centimeters)
		if err != nil {
			return nil, err
		}
		out = append(out, zoneJSON{Key: z.Key, Kind: kind, Pixels: pixels, Centimeters: centimeters})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode zones: %w", err)
	}
	return raw, nil
}

func decodeZones(raw []byte) ([]domain.Zone, error) {
	var stored []zoneJSON
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("decode zones: %w", err)
	}
	out := make([]domain.Zone, 0, len(stored))
	for _, z := range stored {
		pixels, err := decodeShape(z.Kind, z.Pixels)
		if err != nil {
			return nil, err
		}
		centimeters, err := decodeShape(z.Kind, z.Centimeters)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Zone{Key: z.Key, Pixels: pixels, Centimeters: centimeters})
	}
	return out, nil
}

func encodeShape(s domain.Shape) (string, json.RawMessage, error) {
	var kind string
	var v any
	if s.Circle != nil {
		kind, v = "CIRCLE", circleJSON(*s.Circle)
	} else {
		vertices := make([]vertexJSON, 0, len(s.Polygon))
		for _, p := range s.Polygon {
			vertices = append(vertices, vertexJSON(p))
		}
		kind, v = "POLYGON", vertices
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", nil, fmt.Errorf("encode zone shape: %w", err)
	}
	return kind, raw, nil
}

func decodeShape(kind string, raw []byte) (domain.Shape, error) {
	if kind == "CIRCLE" {
		var c circleJSON
		if err := json.Unmarshal(raw, &c); err != nil {
			return domain.Shape{}, fmt.Errorf("decode zone circle: %w", err)
		}
		circle := domain.Circle(c)
		return domain.Shape{Circle: &circle}, nil
	}
	var vertices []vertexJSON
	if err := json.Unmarshal(raw, &vertices); err != nil {
		return domain.Shape{}, fmt.Errorf("decode zone polygon: %w", err)
	}
	out := domain.Shape{Polygon: make([]domain.Vertex, 0, len(vertices))}
	for _, v := range vertices {
		out.Polygon = append(out.Polygon, domain.Vertex(v))
	}
	return out, nil
}
