package core

import (
	"errors"
	"reflect"
	"testing"
)

func TestValidateContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(c *Contract)
		paths  []string // every path must be reported; nil means valid
	}{
		{"valid", func(c *Contract) {}, nil},
		{"valid at controlled-validation with waivers", func(c *Contract) { *c = stageTwoContract() }, nil},
		{"valid with thresholds", func(c *Contract) {
			c.Convergence = &Convergence{RootCauseAfter: 1, ConvergenceAuditAfter: 2, ProcessAuditAfter: 4, SuspendAfter: 9}
		}, nil},
		{"valid waiver beyond coverage", func(c *Contract) {
			c.NotApplicable = []Waiver{{Domain: DomainPerformance, Rationale: "not yet"}}
		}, nil},
		{"wrong schema", func(c *Contract) { c.Schema = "metron.contract/v2" }, []string{"/schema"}},
		{"blank project name", func(c *Contract) { c.Project.Name = " " }, []string{"/project/name"}},
		{"no objective", func(c *Contract) { c.Project.Objectives = nil }, []string{"/project/objectives"}},
		{"blank objective", func(c *Contract) { c.Project.Objectives = []string{"ok", "\t"} }, []string{"/project/objectives/1"}},
		{"blank non-objective", func(c *Contract) { c.Project.NonObjectives = []string{""} }, []string{"/project/non_objectives/0"}},
		{"unknown stage", func(c *Contract) { c.Stage = "beta" }, []string{"/stage"}},
		{"blank authorizes", func(c *Contract) { c.Authorizes = "" }, []string{"/authorizes"}},
		{"empty scope", func(c *Contract) { c.Scope = nil }, []string{"/scope"}},
		{"blank scope", func(c *Contract) { c.Scope = []string{""} }, []string{"/scope/0"}},
		{"empty envelope", func(c *Contract) { c.Envelope = nil }, []string{"/operating_envelope"}},
		{"no requirement", func(c *Contract) { c.Requirements = nil }, []string{"/requirements"}},
		{"malformed requirement id", func(c *Contract) { c.Requirements[1].ID = "REQ-two" }, []string{"/requirements/1/id"}},
		{"duplicate requirement id", func(c *Contract) { c.Requirements[2].ID = "REQ-1" }, []string{"/requirements/2/id"}},
		{"unknown requirement domain", func(c *Contract) { c.Requirements[3].Domain = "vibes" }, []string{"/requirements/3/domain"}},
		{"blank statement and criterion", func(c *Contract) {
			c.Requirements[0].Statement, c.Requirements[0].Criterion = "", " "
		}, []string{"/requirements/0/statement", "/requirements/0/criterion"}},
		{"blank command", func(c *Contract) { c.Requirements[0].Command = "  " }, []string{"/requirements/0/command"}},
		{"unknown from stage", func(c *Contract) { c.Requirements[3].From = "later" }, []string{"/requirements/3/from"}},
		{"no applicable requirement", func(c *Contract) {
			for i := range c.Requirements {
				c.Requirements[i].From = StageProduction
			}
		}, []string{"/requirements"}},
		{"coverage missing", func(c *Contract) { c.Requirements = c.Requirements[:2] }, []string{"/requirements"}},
		{"coverage by waiver", func(c *Contract) {
			c.Requirements = c.Requirements[:2]
			c.NotApplicable = []Waiver{{Domain: DomainArchitecture, Rationale: "a script"}}
		}, nil},
		{"waived domain with applicable requirement", func(c *Contract) {
			c.NotApplicable = []Waiver{{Domain: DomainArchitecture, Rationale: "a script"}}
		}, []string{"/not_applicable/0/domain"}},
		{"duplicate waiver", func(c *Contract) {
			c.NotApplicable = []Waiver{{Domain: DomainPerformance, Rationale: "a"}, {Domain: DomainPerformance, Rationale: "b"}}
		}, []string{"/not_applicable/1/domain"}},
		{"unknown waiver domain and blank rationale", func(c *Contract) {
			c.NotApplicable = []Waiver{{Domain: "nope", Rationale: ""}}
		}, []string{"/not_applicable/0/domain", "/not_applicable/0/rationale"}},
		{"no unacceptable risk", func(c *Contract) { c.Risks.Unacceptable = nil }, []string{"/risks/unacceptable"}},
		{"malformed risk ids", func(c *Contract) {
			c.Risks.Unacceptable = []Risk{{ID: "ACC-1", Statement: "x"}}
			c.Risks.Accepted = []Risk{{ID: "RISK-1", Statement: "y"}}
		}, []string{"/risks/unacceptable/0/id", "/risks/accepted/0/id"}},
		{"duplicate accepted risk and blank statement", func(c *Contract) {
			c.Risks.Accepted = []Risk{{ID: "ACC-1", Statement: "x"}, {ID: "ACC-1", Statement: ""}}
		}, []string{"/risks/accepted/1/id", "/risks/accepted/1/statement"}},
		{"calibration", func(c *Contract) {
			c.Calibrations = []Calibration{{ID: "CAL-1", Parameter: "p", EvidenceNeeded: "e", ReassessWhen: "r"}, {ID: "CAL-1"}}
		}, []string{"/calibrations/1/id", "/calibrations/1/parameter", "/calibrations/1/evidence_needed", "/calibrations/1/reassess_when"}},
		{"thresholds not positive", func(c *Contract) { c.Convergence = &Convergence{} }, []string{
			"/convergence/root_cause_after", "/convergence/convergence_audit_after", "/convergence/process_audit_after", "/convergence/suspend_after", "/convergence"}},
		{"convergence not below process audit", func(c *Contract) {
			c.Convergence = &Convergence{RootCauseAfter: 2, ConvergenceAuditAfter: 5, ProcessAuditAfter: 5, SuspendAfter: 20}
		}, []string{"/convergence"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testContract()
			tc.mutate(&c)
			err := ValidateContract(c)
			if tc.paths == nil {
				if err != nil {
					t.Fatalf("valid contract rejected: %v", err)
				}
				return
			}
			var r *Rejection
			if !errors.As(err, &r) || r.Layer != LayerSemantic {
				t.Fatalf("got %v, want a semantic rejection", err)
			}
			for _, p := range tc.paths {
				if !hasProblem(r.Problems, p) {
					t.Errorf("no problem at %s in %+v", p, r.Problems)
				}
			}
			if again := ValidateContract(c); !reflect.DeepEqual(again, err) {
				t.Error("validation is not deterministic")
			}
		})
	}
}

func TestValidateContractZeroValue(t *testing.T) {
	var r *Rejection
	if err := ValidateContract(Contract{}); !errors.As(err, &r) || len(r.Problems) < 5 {
		t.Fatalf("the zero contract: %v", err)
	}
}
