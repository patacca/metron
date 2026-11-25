package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/schemas"
)

// reportBodies maps a report role to the Go type of its body.
var reportBodies = map[core.Role]reflect.Type{
	core.RoleReviewer:    reflect.TypeOf(core.ReviewerBody{}),
	core.RoleImplementer: reflect.TypeOf(core.ImplementerBody{}),
	core.RoleVerifier:    reflect.TypeOf(core.VerifierBody{}),
	core.RoleAuditor:     reflect.TypeOf(core.AuditorBody{}),
}

// eventPayloads maps every event type to the Go type of its payload.
var eventPayloads = map[core.EventType]reflect.Type{
	core.EventLedgerInitialized:   reflect.TypeOf(core.LedgerInitialized{}),
	core.EventContractPinned:      reflect.TypeOf(core.ContractPinned{}),
	core.EventEvidenceRecorded:    reflect.TypeOf(core.EvidenceRecorded{}),
	core.EventRoundOpened:         reflect.TypeOf(core.RoundOpened{}),
	core.EventPacketIssued:        reflect.TypeOf(core.PacketIssued{}),
	core.EventPacketVoided:        reflect.TypeOf(core.PacketVoided{}),
	core.EventReportAccepted:      reflect.TypeOf(core.ReportAccepted{}),
	core.EventVerdictRecorded:     reflect.TypeOf(core.VerdictRecorded{}),
	core.EventFindingOpened:       reflect.TypeOf(core.FindingOpened{}),
	core.EventFindingFixClaimed:   reflect.TypeOf(core.FindingFixClaimed{}),
	core.EventFindingFixConfirmed: reflect.TypeOf(core.FindingFixConfirmed{}),
	core.EventFindingFixRejected:  reflect.TypeOf(core.FindingFixRejected{}),
	core.EventFindingAddressed:    reflect.TypeOf(core.FindingAddressed{}),
	core.EventFindingReopened:     reflect.TypeOf(core.FindingReopened{}),
	core.EventFindingWithdrawn:    reflect.TypeOf(core.FindingWithdrawn{}),
	core.EventFindingReclassified: reflect.TypeOf(core.FindingReclassified{}),
	core.EventEscalationRaised:    reflect.TypeOf(core.EscalationRaised{}),
	core.EventEscalationResolved:  reflect.TypeOf(core.EscalationResolved{}),
	core.EventDirectiveIssued:     reflect.TypeOf(core.DirectiveIssued{}),
	core.EventDecisionSuperseded:  reflect.TypeOf(core.DecisionSuperseded{}),
}

type node = map[string]any

// reporter is the part of testing.T the mirror needs, so that a test can check
// that the mirror fails when it should.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

type docs map[string]node // file name, such as "defs.schema.json", to document

func loadDocs(t testing.TB) docs {
	t.Helper()
	d := docs{}
	for _, n := range Names() {
		b, err := schemas.FS.ReadFile(dir + n + fileExt)
		if err != nil {
			t.Fatal(err)
		}
		var doc node
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		d[n+fileExt] = doc
	}
	return d
}

// resolve follows $ref chains and returns the node and the file it lives in.
func (d docs) resolve(t reporter, file string, n node) (string, node) {
	t.Helper()
	for {
		ref, ok := n["$ref"].(string)
		if !ok {
			return file, n
		}
		target, frag, _ := strings.Cut(ref, "#")
		if target != "" {
			file = target
		}
		doc, ok := d[file]
		if !ok {
			t.Fatalf("$ref %q in %s: no such embedded schema", ref, file)
		}
		cur := doc
		if frag != "" {
			for _, tok := range strings.Split(strings.TrimPrefix(frag, "/"), "/") {
				next, ok := cur[tok].(node)
				if !ok {
					t.Fatalf("$ref %q in %s: %q not found", ref, file, tok)
				}
				cur = next
			}
		}
		n = cur
	}
}

func (d docs) def(t reporter, file, name string) node {
	t.Helper()
	n, ok := d[file]["$defs"].(node)[name].(node)
	if !ok {
		t.Fatalf("%s has no $defs/%s", file, name)
	}
	return n
}

// jsonFields returns the JSON member names of a struct and which of them are
// required, i.e. not marked omitempty.
func jsonFields(t reporter, typ reflect.Type) (names []string, required map[string]bool, byName map[string]reflect.Type) {
	t.Helper()
	required = map[string]bool{}
	byName = map[string]reflect.Type{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag, ok := f.Tag.Lookup("json")
		if !ok {
			t.Fatalf("%s.%s has no json tag", typ, f.Name)
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" || name == "" {
			t.Fatalf("%s.%s: unsupported json tag %q", typ, f.Name, tag)
		}
		names = append(names, name)
		byName[name] = f.Type
		if !strings.Contains(","+opts+",", ",omitempty,") {
			required[name] = true
		}
	}
	sort.Strings(names)
	return names, required, byName
}

func keysOf(m node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var rawMessage = reflect.TypeOf(json.RawMessage(nil))

// mirror asserts that the schema node describes the Go type: the same member
// names, requiredness, nesting and basic types, recursively. Members whose Go
// type is json.RawMessage are dispatched by role or type and checked
// separately.
func (d docs) mirror(t reporter, path, file string, n node, typ reflect.Type) {
	t.Helper()
	file, n = d.resolve(t, file, n)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == rawMessage {
		return
	}
	want := func(jsonType string) {
		if n["type"] != jsonType {
			t.Errorf("%s: Go type %s needs schema type %q, schema says %v", path, typ, jsonType, n["type"])
		}
	}
	switch typ.Kind() {
	case reflect.Struct:
		want("object")
		props, _ := n["properties"].(node)
		names, required, byName := jsonFields(t, typ)
		if got := keysOf(props); !reflect.DeepEqual(got, names) {
			t.Errorf("%s: schema properties %v differ from the members of Go type %s %v", path, got, typ, names)
		}
		var gotReq []string
		if r, ok := n["required"].([]any); ok {
			for _, v := range r {
				gotReq = append(gotReq, v.(string))
			}
		}
		sort.Strings(gotReq)
		var wantReq []string
		for name := range required {
			wantReq = append(wantReq, name)
		}
		sort.Strings(wantReq)
		if !reflect.DeepEqual(gotReq, wantReq) {
			t.Errorf("%s: schema required %v differ from the members of Go type %s without omitempty %v", path, gotReq, typ, wantReq)
		}
		if n["additionalProperties"] != false {
			t.Errorf("%s: schema of Go type %s must set additionalProperties to false", path, typ)
		}
		for _, name := range names {
			child, ok := props[name].(node)
			if !ok {
				continue // reported above
			}
			d.mirror(t, path+"."+name, file, child, byName[name])
		}
	case reflect.Slice:
		want("array")
		items, ok := n["items"].(node)
		if !ok {
			t.Errorf("%s: array schema for Go type %s has no items", path, typ)
			return
		}
		d.mirror(t, path+"[]", file, items, typ.Elem())
	case reflect.String:
		want("string")
	case reflect.Int:
		want("integer")
	case reflect.Bool:
		want("boolean")
	default:
		t.Errorf("%s: Go kind %s of %s is not supported by the mirror test", path, typ.Kind(), typ)
	}
}

// dispatch returns, for a schema whose allOf holds if/then pairs that select
// property target by the constant value of property key, the then-branch
// schema of each value.
func (d docs) dispatch(t reporter, file, key, target string) map[string]node {
	t.Helper()
	out := map[string]node{}
	all, _ := d[file]["allOf"].([]any)
	for _, a := range all {
		pair := a.(node)
		cond := pair["if"].(node)["properties"].(node)[key].(node)["const"].(string)
		req, _ := pair["if"].(node)["required"].([]any)
		if len(req) != 1 || req[0] != key {
			t.Errorf("%s: dispatch on %s=%s must require %s", file, key, cond, key)
		}
		then := pair["then"].(node)["properties"].(node)
		if !reflect.DeepEqual(keysOf(then), []string{target}) {
			t.Errorf("%s: dispatch on %s=%s must constrain only %s, got %v", file, key, cond, target, keysOf(then))
		}
		if _, dup := out[cond]; dup {
			t.Errorf("%s: dispatch on %s=%s appears twice", file, key, cond)
		}
		out[cond] = then[target].(node)
	}
	return out
}

func TestMirrorGoTypes(t *testing.T) {
	d := loadDocs(t)
	d.mirror(t, "Contract", "contract.schema.json", d["contract.schema.json"], reflect.TypeOf(core.Contract{}))
	d.mirror(t, "Packet", "packet.schema.json", d["packet.schema.json"], reflect.TypeOf(core.Packet{}))
	d.mirror(t, "Report", "report.schema.json", d["report.schema.json"], reflect.TypeOf(core.Report{}))
	d.mirror(t, "Event", "event.schema.json", d["event.schema.json"], reflect.TypeOf(core.Event{}))

	bodies := d.dispatch(t, "report.schema.json", "role", "body")
	if len(bodies) != len(reportBodies) {
		t.Errorf("report schema dispatches %d roles, core has %d report bodies", len(bodies), len(reportBodies))
	}
	for role, typ := range reportBodies {
		n, ok := bodies[string(role)]
		if !ok {
			t.Errorf("report schema does not dispatch role %q to %s", role, typ)
			continue
		}
		d.mirror(t, fmt.Sprintf("Report(role=%s).body", role), "report.schema.json", n, typ)
	}

	payloads := d.dispatch(t, "event.schema.json", "type", "data")
	if len(payloads) != len(eventPayloads) {
		t.Errorf("event schema dispatches %d types, core has %d payloads", len(payloads), len(eventPayloads))
	}
	for typ, goType := range eventPayloads {
		n, ok := payloads[string(typ)]
		if !ok {
			t.Errorf("event schema does not dispatch type %q to %s", typ, goType)
			continue
		}
		d.mirror(t, fmt.Sprintf("Event(type=%s).data", typ), "event.schema.json", n, goType)
	}
}

// The mirror test is only worth something if it can fail: check that it does.
func TestMirrorDetectsDrift(t *testing.T) {
	d := loadDocs(t)
	type sample struct {
		Name  string `json:"name"`
		Count int    `json:"count,omitempty"`
	}
	schema := func(props node, req []any) node {
		return node{"type": "object", "properties": props, "required": req, "additionalProperties": false}
	}
	str, num := node{"type": "string"}, node{"type": "integer"}
	cases := []struct {
		name string
		n    node
		want string
	}{
		{"extra property", schema(node{"name": str, "count": num, "more": str}, []any{"name"}), "differ from the members"},
		{"missing property", schema(node{"name": str}, []any{"name"}), "differ from the members"},
		{"requiredness", schema(node{"name": str, "count": num}, []any{"name", "count"}), "without omitempty"},
		{"wrong type", schema(node{"name": num, "count": num}, []any{"name"}), `needs schema type "string"`},
	}
	for _, tc := range cases {
		rec := &recorder{}
		d.mirror(rec, "Sample", "defs.schema.json", tc.n, reflect.TypeOf(sample{}))
		if !strings.Contains(rec.msg, tc.want) || !strings.Contains(rec.msg, "Sample") {
			t.Errorf("%s: drift not reported as %q: %q", tc.name, tc.want, rec.msg)
		}
	}
}

// recorder collects failures instead of failing the test.
type recorder struct{ msg string }

func (r *recorder) Helper()                        {}
func (r *recorder) Errorf(format string, a ...any) { r.msg += fmt.Sprintf(format, a...) + "\n" }
func (r *recorder) Fatalf(format string, a ...any) { r.msg += fmt.Sprintf(format, a...) + "\n" }

func strs(vals []any) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = fmt.Sprint(v)
	}
	sort.Strings(out)
	return out
}

func sorted[T ~string](vals []T) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = string(v)
	}
	sort.Strings(out)
	return out
}

func TestEnumerations(t *testing.T) {
	d := loadDocs(t)
	enumOf := func(n node) []string {
		_, ok := n["enum"].([]any)
		if !ok {
			t.Fatalf("schema node %v has no enum", n)
		}
		return strs(n["enum"].([]any))
	}
	constOf := func(n node) string {
		c, ok := n["const"].(string)
		if !ok {
			t.Fatalf("schema node %v has no const", n)
		}
		return c
	}
	prop := func(n node, name string) node { return n["properties"].(node)[name].(node) }
	check := func(name string, got, want []string) {
		t.Helper()
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: schema has %v, Go constants are %v", name, got, want)
		}
	}
	ordered := func(name string, n node, want []string) {
		t.Helper()
		got := make([]string, 0)
		for _, v := range n["enum"].([]any) {
			got = append(got, v.(string))
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: schema order %v, Go order %v", name, got, want)
		}
	}
	defs := func(name string) node { return d.def(t, "defs.schema.json", name) }

	var stages, tiers, domains []string
	for _, v := range core.Stages() {
		stages = append(stages, string(v))
	}
	for _, v := range core.Tiers() {
		tiers = append(tiers, string(v))
	}
	for _, v := range core.Domains() {
		domains = append(domains, string(v))
	}
	ordered("stage", defs("stage"), stages)
	ordered("tier", defs("tier"), tiers)
	ordered("domain", defs("domain"), domains)
	check("stage", enumOf(defs("stage")), append([]string(nil), stages...))
	check("tier", enumOf(defs("tier")), append([]string(nil), tiers...))
	check("domain", enumOf(defs("domain")), append([]string(nil), domains...))

	check("verdict", enumOf(defs("verdict")), sorted([]core.Verdict{core.VerdictGreen, core.VerdictRed, core.VerdictIndeterminate}))
	roles := []core.Role{core.RoleGovernance, core.RoleImplementer, core.RoleReviewer, core.RoleVerifier, core.RoleAuditor}
	check("role", enumOf(defs("role")), sorted(roles))
	for _, r := range roles {
		if !r.Valid() {
			t.Errorf("test lists %q as a role but core disagrees", r)
		}
	}

	steps := []core.Step{core.StepRepairLedger, core.StepPinContract, core.StepSubmitReport, core.StepAdjudicate,
		core.StepAudit, core.StepVerify, core.StepFix, core.StepCommitChanges, core.StepRunChecks, core.StepReview, core.StepNone}
	var actions []core.Step
	var packetRoles []core.Role
	for _, s := range steps {
		if r, ok := s.PacketRole(); ok {
			actions = append(actions, s)
			packetRoles = append(packetRoles, r)
		}
	}
	check("packetAction", enumOf(defs("packetAction")), sorted(actions))
	check("packetRole", enumOf(defs("packetRole")), sorted(packetRoles))
	check("report role", enumOf(prop(d["report.schema.json"], "role")), sorted(packetRoles))

	check("findingStatus", enumOf(defs("findingStatus")),
		sorted([]core.FindingStatus{core.StatusOpen, core.StatusClaimed, core.StatusResolved, core.StatusWithdrawn}))
	check("reopenReason", enumOf(defs("reopenReason")), sorted([]core.ReopenReason{core.ReopenNewEvidence,
		core.ReopenRegression, core.ReopenInvalidVerification, core.ReopenRequirementChanged, core.ReopenContractConflict}))
	kinds := []core.EscalationKind{core.EscalationDispute, core.EscalationContradiction, core.EscalationComponentChurn,
		core.EscalationRegressionSpiral, core.EscalationNonConvergence, core.EscalationProcessAudit,
		core.EscalationContractGap, core.EscalationEvidenceGap, core.EscalationSuspension}
	check("escalationKind", enumOf(defs("escalationKind")), sorted(kinds))
	for _, k := range kinds {
		if k.Owner() == "" {
			t.Errorf("test lists %q as an escalation kind but core disagrees", k)
		}
	}
	check("fixScope", enumOf(defs("fixScope")), []string{core.ScopeExcessive, core.ScopeProportionate})
	check("requirementStatus", enumOf(defs("requirementStatus")),
		[]string{core.RequirementSatisfied, core.RequirementUnverified, core.RequirementViolated})
	check("author kind", enumOf(prop(defs("author"), "kind")), []string{core.AuthorAgent, core.AuthorHuman})

	rep := func(name string) node { return d.def(t, "report.schema.json", name) }
	check("coverage status", enumOf(prop(rep("domainAssessment"), "status")), []string{core.CoverageAssessed, core.CoverageNotApplicable})
	check("verification result", enumOf(prop(rep("verification"), "result")), []string{core.VerificationConfirmed, core.VerificationRejected})
	check("adjudication decision", enumOf(prop(rep("adjudication"), "decision")),
		[]string{core.DecisionUphold, core.DecisionWithdraw, core.DecisionReclassify})
	check("decision source", enumOf(prop(d.def(t, "packet.schema.json", "decision"), "source")),
		[]string{core.DecisionDirective, core.DecisionResolution})

	if got := constOf(prop(d.def(t, "event.schema.json", "provenance"), "mode")); got != core.ProvenanceDeclared {
		t.Errorf("provenance mode const = %q, want %q", got, core.ProvenanceDeclared)
	}
	if got := constOf(defs("protocol")); got != core.ProtocolVersion {
		t.Errorf("protocol const = %q, want %q", got, core.ProtocolVersion)
	}
	for file, want := range map[string]string{
		"contract.schema.json": core.SchemaContract,
		"report.schema.json":   core.SchemaReport,
		"packet.schema.json":   core.SchemaPacket,
	} {
		if got := constOf(prop(d[file], "schema")); got != want {
			t.Errorf("%s: schema const = %q, want %q", file, got, want)
		}
	}

	var types []core.EventType
	for typ := range eventPayloads {
		types = append(types, typ)
	}
	check("event type", enumOf(defs("eventType")), sorted(types))
	if ref := prop(d["event.schema.json"], "type")["$ref"]; ref != "defs.schema.json#/$defs/eventType" {
		t.Errorf("event type must reference defs eventType, got %v", ref)
	}
}

// walk calls f for every schema object reachable from n, with a path for
// messages.
func walk(path string, v any, f func(path string, n node)) {
	switch x := v.(type) {
	case node:
		f(path, x)
		for _, k := range keysOf(x) {
			if k == "enum" || k == "const" || k == "required" {
				continue
			}
			walk(path+"/"+k, x[k], f)
		}
	case []any:
		for i, e := range x {
			walk(fmt.Sprintf("%s/%d", path, i), e, f)
		}
	}
}

func TestSchemaConventions(t *testing.T) {
	d := loadDocs(t)
	files := make([]string, 0, len(d))
	for file := range d {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		doc := d[file]
		if doc["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Errorf("%s: $schema is %v", file, doc["$schema"])
		}
		if doc["$id"] != idPrefix+file {
			t.Errorf("%s: $id is %v", file, doc["$id"])
		}
		walk(file, doc, func(path string, n node) {
			if ref, ok := n["$ref"].(string); ok && (strings.Contains(ref, "://") || strings.HasPrefix(ref, "/")) {
				t.Errorf("%s: $ref %q is not relative", path, ref)
			}
			// The if/then branches of a dispatch constrain one member of an
			// object whose own schema already forbids unknown members.
			dispatchBranch := strings.Contains(path, "/allOf/") && (strings.HasSuffix(path, "/if") || strings.HasSuffix(path, "/then"))
			if _, ok := n["properties"]; ok && !dispatchBranch && n["additionalProperties"] != false {
				t.Errorf("%s: object with properties must set additionalProperties to false", path)
			}
			if p, ok := n["pattern"].(string); ok {
				if _, ok := patternHints[p]; !ok {
					t.Errorf("%s: pattern %q has no message in patternHints", path, p)
				}
			}
			if n["type"] == "number" {
				t.Errorf("%s: numbers must be integers", path)
			}
		})
	}
}

// The validator ignores keywords it does not know, so a misplaced or
// misspelled one would silently weaken a schema. Check every schema object
// against the list of keywords the schemas use.
var schemaKeywords = map[string]bool{
	"$schema": true, "$id": true, "$ref": true, "$defs": true, "title": true, "description": true,
	"type": true, "properties": true, "required": true, "additionalProperties": true, "items": true,
	"minItems": true, "enum": true, "const": true, "pattern": true, "minimum": true,
	"allOf": true, "if": true, "then": true,
}

func checkKeywords(t reporter, path string, n node) {
	for _, k := range keysOf(n) {
		if !schemaKeywords[k] {
			t.Errorf("%s: %q is not a keyword the Metron schemas use", path, k)
		}
	}
	for _, k := range []string{"properties", "$defs"} {
		if m, ok := n[k].(node); ok {
			for _, name := range keysOf(m) {
				child, ok := m[name].(node)
				if !ok {
					t.Errorf("%s/%s/%s: not a schema object", path, k, name)
					continue
				}
				checkKeywords(t, path+"/"+k+"/"+name, child)
			}
		}
	}
	for _, k := range []string{"items", "if", "then", "additionalProperties"} {
		if child, ok := n[k].(node); ok {
			checkKeywords(t, path+"/"+k, child)
		}
	}
	if all, ok := n["allOf"].([]any); ok {
		for i, a := range all {
			if child, ok := a.(node); ok {
				checkKeywords(t, fmt.Sprintf("%s/allOf/%d", path, i), child)
			}
		}
	}
}

func TestNoUnknownKeywords(t *testing.T) {
	d := loadDocs(t)
	files := make([]string, 0, len(d))
	for file := range d {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		checkKeywords(t, file, d[file])
	}
	rec := &recorder{}
	checkKeywords(rec, "sample", node{"type": "object", "role": node{}})
	if !strings.Contains(rec.msg, `"role" is not a keyword`) {
		t.Errorf("a stray keyword went unreported: %q", rec.msg)
	}
}
