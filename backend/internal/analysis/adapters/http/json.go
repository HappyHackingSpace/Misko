package analysishttp

import (
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

type dataJSON struct {
	Data any `json:"data"`
}

type capabilityJSON struct {
	ParadigmKey     string `json:"paradigmKey"`
	ParadigmVersion int    `json:"paradigmVersion"`
}

type workerJSON struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	ModelVersion string           `json:"modelVersion"`
	Capabilities []capabilityJSON `json:"capabilities"`
	DisabledAt   *time.Time       `json:"disabledAt"`
	CreatedAt    time.Time        `json:"createdAt"`
}

func toWorker(w domain.Worker) workerJSON {
	return workerJSON{w.ID, w.Name, w.ModelVersion, mapSlice(w.Capabilities, func(c domain.Capability) capabilityJSON { return capabilityJSON(c) }), w.DisabledAt, w.CreatedAt}
}

func capabilities(in []capabilityJSON) []domain.Capability {
	out := make([]domain.Capability, 0, len(in))
	for _, c := range in {
		out = append(out, domain.Capability(c))
	}
	return out
}

type requesterJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// jobJSON is a job's snapshot and execution state. The calibration snapshot
// is in calibration; calibrationId names where it was copied from.
type jobJSON struct {
	ID                  string             `json:"id"`
	Requester           requesterJSON      `json:"requester"`
	ParadigmKey         string             `json:"paradigmKey"`
	ParadigmVersion     int                `json:"paradigmVersion"`
	MetricEngineVersion int                `json:"metricEngineVersion"`
	ResultSchemaVersion int                `json:"resultSchemaVersion"`
	Parameters          map[string]float64 `json:"parameters"`
	SourceVideoID       string             `json:"sourceVideoId"`
	SourceGeneration    string             `json:"sourceGeneration"`
	SourceCRC32C        string             `json:"sourceCrc32c"`
	ClipStartUs         int64              `json:"clipStartUs"`
	ClipEndUs           *int64             `json:"clipEndUs"`
	CalibrationID       *string            `json:"calibrationId"`
	Calibration         *calibrationJSON   `json:"calibration"`
	Trigger             string             `json:"trigger"`
	Status              string             `json:"status"`
	Attempt             int                `json:"attempt"`
	MaxAttempts         int                `json:"maxAttempts"`
	WorkerID            *string            `json:"workerId"`
	LeaseExpiresAt      *time.Time         `json:"leaseExpiresAt"`
	ModelVersion        *string            `json:"modelVersion"`
	FailureReason       *string            `json:"failureReason"`
	CreatedBy           *string            `json:"createdBy"`
	CreatedAt           time.Time          `json:"createdAt"`
	FinishedAt          *time.Time         `json:"finishedAt"`
}

func toJob(j domain.Job) jobJSON {
	params := j.Parameters
	if params == nil {
		params = map[string]float64{}
	}
	return jobJSON{
		ID: j.ID, Requester: requesterJSON{string(j.Requester.Kind), j.Requester.ID}, ParadigmKey: j.ParadigmKey, ParadigmVersion: j.ParadigmVersion,
		MetricEngineVersion: j.MetricEngineVersion, ResultSchemaVersion: j.ResultSchemaVersion, Parameters: params,
		SourceVideoID: j.Source.AssetID, SourceGeneration: itoa(j.Source.Generation), SourceCRC32C: formatCRC32C(j.Source.CRC32C),
		ClipStartUs: j.Source.ClipStartUs, ClipEndUs: j.Source.ClipEndUs, CalibrationID: nullable(j.CalibrationID()), Calibration: toCalibration(j.Calibration),
		Trigger: string(j.Trigger), Status: string(j.Status), Attempt: j.Attempt, MaxAttempts: j.MaxAttempts, WorkerID: nullable(j.WorkerID),
		LeaseExpiresAt: j.LeaseExpiresAt, ModelVersion: nullable(j.ModelVersion), FailureReason: nullable(j.FailureReason),
		CreatedBy: nullable(j.CreatedBy), CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
	}
}

// claimJSON is the job a worker leased. run is the job under the name
// workers read; testSubject names the animal of a test subject job, for logs.
type claimJSON struct {
	Run              jobJSON          `json:"run"`
	Job              jobJSON          `json:"job"`
	Requester        requesterJSON    `json:"requester"`
	TestSubject      *testSubjectJSON `json:"testSubject"`
	OutputPrefix     string           `json:"outputPrefix"`
	TrajectorySchema string           `json:"trajectorySchema"`
	Contract         contractJSON     `json:"contract"`
	Calibration      *calibrationJSON `json:"calibration"`
}

type testSubjectJSON struct {
	ID string `json:"id"`
}

func toClaim(c application.Claimed) claimJSON {
	job := toJob(c.Job)
	out := claimJSON{Run: job, Job: job, Requester: job.Requester, OutputPrefix: c.Job.OutputPrefix(), TrajectorySchema: domain.TrajectorySchema,
		Contract: toContract(c.Contract), Calibration: job.Calibration}
	if c.Job.Requester.Kind == domain.RequesterTestSubject {
		out.TestSubject = &testSubjectJSON{ID: c.Job.Requester.ID}
	}
	return out
}

type metricSpecJSON struct {
	Key     string  `json:"key"`
	Unit    string  `json:"unit"`
	Integer bool    `json:"integer"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

type eventSpecJSON struct {
	Type string `json:"type"`
	Kind string `json:"kind"`
}

type qcRuleJSON struct {
	Key      string  `json:"key"`
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
}

// contractJSON lists what the engine will compute; workers send trajectories, not metrics.
type contractJSON struct {
	// RequiredZones are the calibrated zones the job's calibration carries.
	RequiredZones  []string         `json:"requiredZones"`
	Metrics        []metricSpecJSON `json:"metrics"`
	Events         []eventSpecJSON  `json:"events"`
	MissingReasons []string         `json:"missingReasons"`
	QC             []qcRuleJSON     `json:"qc"`
}

func toContract(c domain.Contract) contractJSON {
	out := contractJSON{RequiredZones: append([]string{}, c.RequiredZones...), MissingReasons: c.MissingReasons, Metrics: []metricSpecJSON{},
		Events: []eventSpecJSON{}, QC: []qcRuleJSON{}}
	for _, m := range c.Metrics {
		out.Metrics = append(out.Metrics, metricSpecJSON{m.Key, m.Unit, m.Integer, m.Min, m.Max})
	}
	for _, e := range c.Events {
		out.Events = append(out.Events, eventSpecJSON{e.Type, e.Kind})
	}
	for _, q := range c.QC {
		out.QC = append(out.QC, qcRuleJSON(q))
	}
	return out
}

type cropJSON struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type calibrationJSON struct {
	ID               string     `json:"id"`
	FrameWidth       int        `json:"frameWidth"`
	FrameHeight      int        `json:"frameHeight"`
	Crop             cropJSON   `json:"crop"`
	MeasurementPlane string     `json:"measurementPlane"`
	Transform        [9]float64 `json:"transform"`
	Zones            []zoneJSON `json:"zones"`
}

func toCalibration(c *domain.Calibration) *calibrationJSON {
	if c == nil {
		return nil
	}
	return &calibrationJSON{ID: c.ID, FrameWidth: c.FrameWidth, FrameHeight: c.FrameHeight, Crop: cropJSON(c.Crop), MeasurementPlane: c.Plane,
		Transform: c.Transform, Zones: mapSlice(c.Zones, func(z domain.Zone) zoneJSON {
			return zoneJSON{Key: z.Key, Pixels: toShape(z.Pixels), Centimeters: toShape(z.Centimeters)}
		})}
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

// shapeJSON is a circle or a polygon; exactly one is set.
type shapeJSON struct {
	Circle  *circleJSON  `json:"circle,omitempty"`
	Polygon []vertexJSON `json:"polygon,omitempty"`
}

// zoneJSON is one calibrated zone shape in full-frame pixels and in the
// calibration's centimetres.
type zoneJSON struct {
	Key         string    `json:"key"`
	Pixels      shapeJSON `json:"pixels"`
	Centimeters shapeJSON `json:"centimeters"`
}

func toShape(s domain.Shape) shapeJSON {
	out := shapeJSON{}
	if s.Circle != nil {
		c := circleJSON(*s.Circle)
		out.Circle = &c
	}
	for _, v := range s.Polygon {
		out.Polygon = append(out.Polygon, vertexJSON(v))
	}
	return out
}

type metricJSON struct {
	Key           string   `json:"key"`
	Unit          string   `json:"unit"`
	Value         *float64 `json:"value"`
	MissingReason *string  `json:"missingReason"`
}

type eventJSON struct {
	Type       string  `json:"type"`
	Label      *string `json:"label"`
	Kind       string  `json:"kind"`
	StartUs    int64   `json:"startUs"`
	EndUs      int64   `json:"endUs"`
	Confidence float64 `json:"confidence"`
}

type artifactJSON struct {
	Kind        string `json:"kind"`
	ObjectName  string `json:"objectName"`
	Generation  string `json:"generation"`
	SizeBytes   int64  `json:"sizeBytes"`
	ContentType string `json:"contentType"`
}

type pairJSON struct {
	AnalyzedObjectName string `json:"analyzedObjectName"`
	SourceOffsetUs     int64  `json:"sourceOffsetUs"`
	OutputOffsetUs     int64  `json:"outputOffsetUs"`
	TimeMappingVersion string `json:"timeMappingVersion"`
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

// resultJSON is what a job published; qc is also set when QC failed the job.
type resultJSON struct {
	Metrics   []metricJSON   `json:"metrics"`
	Events    []eventJSON    `json:"events"`
	Artifacts []artifactJSON `json:"artifacts"`
	Pair      *pairJSON      `json:"pair"`
	QC        *qcJSON        `json:"qc"`
}

func toResult(p application.Published) *resultJSON {
	out := &resultJSON{Metrics: []metricJSON{}, Events: []eventJSON{}, Artifacts: []artifactJSON{}}
	for _, m := range p.Metrics {
		out.Metrics = append(out.Metrics, metricJSON{m.Key, m.Unit, m.Value, nullable(m.MissingReason)})
	}
	for _, e := range p.Events {
		out.Events = append(out.Events, eventJSON{e.Type, nullable(e.Label), e.Kind, e.StartUs, e.EndUs, e.Confidence})
	}
	for _, a := range p.Artifacts {
		out.Artifacts = append(out.Artifacts, artifactJSON{a.Kind, a.ObjectName, itoa(a.Generation), a.SizeBytes, a.ContentType})
	}
	if p.Pair != nil {
		out.Pair = &pairJSON{p.Pair.AnalyzedObjectName, p.Pair.SourceOffsetUs, p.Pair.OutputOffsetUs, p.Pair.TimeMappingVersion}
	}
	if q := p.QC; q != nil {
		out.QC = &qcJSON{q.Samples, q.Tracked, q.Inferred, q.LostRatio, q.InferredRatio, q.MeanConfidence, q.Passed}
	}
	return out
}

type jobDetailJSON struct {
	jobJSON
	Result *resultJSON `json:"result"`
}

func toJobDetail(r application.Result) jobDetailJSON {
	out := jobDetailJSON{jobJSON: toJob(r.Job)}
	if r.Published != nil {
		out.Result = toResult(*r.Published)
	}
	return out
}

type videoPairJSON struct {
	OriginalURL        string    `json:"originalUrl"`
	AnalyzedURL        string    `json:"analyzedUrl"`
	ExpiresAt          time.Time `json:"expiresAt"`
	SourceVideoID      string    `json:"sourceVideoId"`
	SourceGeneration   string    `json:"sourceGeneration"`
	AnalyzedGeneration string    `json:"analyzedGeneration"`
	SourceOffsetUs     int64     `json:"sourceOffsetUs"`
	OutputOffsetUs     int64     `json:"outputOffsetUs"`
	TimeMappingVersion string    `json:"timeMappingVersion"`
}

func toVideoPair(l application.VideoPairLinks) videoPairJSON {
	return videoPairJSON{l.OriginalURL, l.AnalyzedURL, l.ExpiresAt, l.Job.Source.AssetID, itoa(l.Job.Source.Generation), itoa(l.Pair.AnalyzedGeneration),
		l.Pair.SourceOffsetUs, l.Pair.OutputOffsetUs, l.Pair.TimeMappingVersion}
}

type readLinkJSON struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type signedRequestJSON struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

func mapSlice[T, U any](items []T, convert func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

// Generations exceed the integers JavaScript represents exactly, so they are strings.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseCRC32C reads the base64 encoding of a big-endian CRC32C, as storage reports it.
func parseCRC32C(raw string) (uint32, bool) {
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(b) != 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(b), true
}

func formatCRC32C(v uint32) string {
	return base64.StdEncoding.EncodeToString(binary.BigEndian.AppendUint32(nil, v))
}
