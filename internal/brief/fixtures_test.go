package brief

import (
	"slices"

	"github.com/patacca/metron/internal/core"
)

// The packets below are what Metron would issue at four points in the life of
// one fictional project. They are built by hand so that the golden files show
// every part of a brief and a prompt.

const (
	commitRound1 = "4a1c9e2b7d3f60812e5a9c4b7d1e3f2a6b8c0d94"
	commitRound2 = "9b2d4f6a8c0e1357924680ace13579bdf0246813"
	commitBatch  = "b8d0f2a4c6e81357924680ace13579bdf0246802"
	commitFixed  = "c3e5a7b9d1f30246813579bdf02468ace1357902"
	commitRound5 = "e7f9a1b3c5d70246813579bdf02468ace1357924"

	digestRound1 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digestFirst  = "sha256:6666666666666666666666666666666666666666666666666666666666666666"
	digestRound2 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	digestBatch  = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
	digestFixed  = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	digestRound5 = "sha256:5555555555555555555555555555555555555555555555555555555555555555"

	// earlierContract is the contract of an earlier pin, which F-0001 was
	// established under.
	earlierContract = "sha256:9999999999999999999999999999999999999999999999999999999999999999"

	packetDigest   = "sha256:efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef"
	contractDigest = "sha256:abababababababababababababababababababababababababababababababab"
	outputDigest   = "sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"
)

func fullContract() core.Contract {
	return core.Contract{
		Schema: core.SchemaContract,
		Project: core.Project{
			Name: "Courier",
			Objectives: []string{
				"Deliver every accepted webhook event to every matching subscriber.",
				"Let subscribers filter the events they receive by type.",
			},
			NonObjectives: []string{"Exactly-once delivery."},
		},
		Stage:      core.StagePilot,
		Authorizes: "A four-week pilot with two internal teams as subscribers, on staging traffic only.",
		Scope:      []string{"cmd/courier", "internal/delivery", "internal/store"},
		Envelope: []string{
			"Up to 50 events per second, sustained.",
			"At most 200 subscribers.",
			"A single instance; restarts are allowed at any time.",
		},
		Requirements: []core.Requirement{
			{ID: "REQ-1", Domain: core.DomainFunctionalCorrectness,
				Statement: "Every accepted event is delivered at least once to every matching subscriber.",
				Criterion: "The delivery test over 1000 events and 20 subscribers reports no missing delivery.",
				Command:   "go test ./internal/delivery -run TestAtLeastOnce"},
			{ID: "REQ-2", Domain: core.DomainObjectiveAlignment,
				Statement: "A subscriber receives only the event types it subscribed to.",
				Criterion: "Filtering is applied before delivery for every subscription, as shown by the routing tests and by inspection."},
			{ID: "REQ-3", Domain: core.DomainArchitecture,
				Statement: "Delivery, storage and the HTTP surface are separate packages with one-way dependencies.",
				Criterion: "No import cycle, and internal/store imports neither of the other two."},
			{ID: "REQ-4", Domain: core.DomainDataIntegrity, From: core.StageControlledValidation,
				Statement: "An accepted event survives a crash of the process.",
				Criterion: "Killing the process at each write boundary loses no accepted event.",
				Command:   "go test ./internal/store -run TestCrashRecovery"},
			{ID: "REQ-5", Domain: core.DomainTestability, From: core.StageControlledValidation,
				Statement: "Delivery behaviour can be tested without a network.",
				Criterion: "Every delivery test runs against the in-process transport."},
			{ID: "REQ-6", Domain: core.DomainObservability, From: core.StageControlledValidation,
				Statement: "Every failed delivery attempt is logged with the event, the subscriber and the cause.",
				Criterion: "The log of a run with injected failures has one structured entry per failed attempt."},
			{ID: "REQ-7", Domain: core.DomainStability, From: core.StageControlledValidation,
				Statement: "Memory use stays bounded under sustained load inside the envelope.",
				Criterion: "Resident memory after one hour at 50 events per second is within 10% of that after five minutes."},
			{ID: "REQ-8", Domain: core.DomainReliability, From: core.StagePilot,
				Statement: "A subscriber that is down receives its events once it recovers.",
				Criterion: "Retries with backoff continue for at least one hour, and the recovery test passes.",
				Command:   "go test ./internal/delivery -run TestSubscriberRecovery"},
			{ID: "REQ-9", Domain: core.DomainPerformance, From: core.StagePilot,
				Statement: "Delivery latency stays low at the envelope's load.",
				Criterion: "The 99th percentile from acceptance to first attempt is under 500 ms at 50 events per second.",
				Command:   "go test ./internal/delivery -run TestLatencyBudget"},
			{ID: "REQ-10", Domain: core.DomainSecurityPrivacy, From: core.StagePilot,
				Statement: "Every delivery is signed so that a subscriber can authenticate it.",
				Criterion: "Each request carries an HMAC-SHA256 signature over the body with the subscriber's secret."},
			{ID: "REQ-11", Domain: core.DomainScalability, From: core.StageProduction,
				Statement: "Several instances can share the delivery load.",
				Criterion: "Two instances deliver 100 events per second without duplicate work beyond retries."},
			{ID: "REQ-12", Domain: core.DomainOperationalReadiness, From: core.StageProduction,
				Statement: "A deployment can be rolled back without losing queued events.",
				Criterion: "The rollback drill loses no accepted event."},
		},
		Risks: core.Risks{
			Unacceptable: []core.Risk{
				{ID: "RISK-1", Statement: "An event is delivered to a subscriber that did not subscribe to its type."},
				{ID: "RISK-2", Statement: "A subscriber secret appears in logs or error messages."},
			},
			Accepted: []core.Risk{
				{ID: "ACC-1", Statement: "A subscriber may receive the same event more than once."},
			},
		},
		NotApplicable: []core.Waiver{
			{Domain: core.DomainCompatibility, Rationale: "First release; there are no existing subscribers or stored data to stay compatible with."},
		},
		Calibrations: []core.Calibration{
			{ID: "CAL-1", Parameter: "Retry backoff starting at 2 s and capped at 5 min",
				EvidenceNeeded: "Recovery times of real subscribers during the pilot.",
				ReassessWhen:   "Two weeks of pilot traffic have been delivered."},
		},
	}
}

func applicableIDs(c core.Contract) []string {
	var ids []string
	for _, r := range c.Applicable() {
		ids = append(ids, r.ID)
	}
	return ids
}

func passed(req, command, candidate string) core.Evidence {
	return core.Evidence{Requirement: req, Command: command, Candidate: candidate,
		ExitCode: 0, Passed: true, OutputDigest: outputDigest}
}

// The findings follow one timeline. Round 1 (review P-0001) opens F-0001 to
// F-0004 and reports a contract gap, E-0001, which governance dismisses. Fix
// P-0002 fixes F-0001 and F-0004 and disputes F-0002 (E-0002); audit P-0003
// reclassifies F-0002 and issues a directive. Verify P-0004 confirms F-0001
// and rejects F-0004, fix P-0005 fixes F-0004 again and verify P-0006
// confirms it. Round 2 (review P-0007) reopens F-0004 and opens F-0005 to
// F-0007. Fix P-0008 fixes F-0004 and disputes F-0005 (E-0003) and F-0007
// (E-0004); audit P-0009 upholds F-0005 and withdraws F-0007; verify P-0010
// rejects the F-0004 fix. Fix P-0011, verify P-0012 and review P-0013 are the
// golden packets. The audit packet, P-0021, comes from round 5 of the same
// project.

// stepCandidates names the candidate each packet's report was made on: the
// one a read-only role examined, or the one an implementer claimed its fixes
// on.
var stepCandidates = map[string]string{
	"P-0001": digestRound1,
	"P-0002": digestFirst, "P-0003": digestFirst, "P-0004": digestFirst,
	"P-0005": digestRound2, "P-0006": digestRound2, "P-0007": digestRound2,
	"P-0008": digestBatch, "P-0009": digestBatch, "P-0010": digestBatch,
	"P-0011": digestFixed, "P-0012": digestFixed, "P-0017": digestFixed,
	"P-0019": digestRound5,
}

func step(event core.EventType, round int, packet, note string) core.FindingEvent {
	e := core.FindingEvent{Event: event, Round: round, Packet: packet, Note: note}
	// A dispute comes with an implementer report and is bound to no candidate.
	if role, _ := core.StepFix.PacketRole(); event != core.EventEscalationRaised || packetRoles[packet] != role {
		e.Candidate = stepCandidates[packet]
	}
	return e
}

// packetRoles names the role of the packets whose reports raised escalations.
var packetRoles = map[string]core.Role{
	"P-0002": core.RoleImplementer, "P-0008": core.RoleImplementer, "P-0020": core.RoleImplementer,
	"P-0019": core.RoleReviewer,
}

func f1Resolved() core.Finding {
	return core.Finding{
		ID: "F-0001", Status: core.StatusResolved, Tier: core.TierMaterial, Round: 1, Source: core.RoleReviewer,
		Input: core.FindingInput{
			Key: "retry-stops", Tier: core.TierMaterial, Domain: core.DomainReliability,
			Title:       "Retries stop after the fifth failed attempt",
			Components:  []string{"internal/delivery/retry.go"},
			Description: "The retry loop counts attempts instead of elapsed time and gives up after five, about 62 s with the current backoff.",
			Evidence:    "TestSubscriberRecovery with a 10-minute outage fails: no delivery after recovery.",
			Impact:      "Any subscriber outage longer than about a minute loses events, which pilot subscribers will hit.",
			Requirement: "REQ-8", BlockingRationale: "The pilot depends on subscribers that restart during deployments.",
			Verification: "TestSubscriberRecovery passes with a one-hour outage.",
		},
		Fix: &core.Fix{Finding: "F-0001", Commits: []string{commitRound2}, Summary: "Retry until one hour has elapsed since the first attempt.",
			Components: []string{"internal/delivery/retry.go"}, Tests: "Extended TestSubscriberRecovery to a one-hour simulated outage."},
		History: []core.FindingEvent{
			step(core.EventFindingOpened, 1, "P-0001", ""),
			step(core.EventFindingFixClaimed, 1, "P-0002", "Retry until one hour has elapsed since the first attempt."),
			step(core.EventFindingFixConfirmed, 1, "P-0004", "TestSubscriberRecovery passes with a one-hour simulated outage; the loop now compares elapsed time, not attempts."),
		},
	}
}

func f2Reclassified() core.Finding {
	return core.Finding{
		ID: "F-0002", Status: core.StatusOpen, Tier: core.TierImprovement, Round: 1, Source: core.RoleReviewer, Disputed: true,
		Input: core.FindingInput{
			Key: "batch-writes", Tier: core.TierMaterial, Domain: core.DomainPerformance,
			Title:       "Each accepted event is written with its own fsync",
			Components:  []string{"internal/store/append.go"},
			Description: "Appends are synced one at a time, which caps throughput at the disk's sync rate.",
			Evidence:    "A benchmark on the reference machine reaches 400 appends per second.",
			Impact:      "Throughput headroom above the envelope's 50 events per second is smaller than it could be.",
			Requirement: "REQ-9", BlockingRationale: "Latency could exceed the budget under bursts.",
			Verification: "TestLatencyBudget passes at 200 events per second.",
			Proportionality: &core.Proportionality{
				ProposedChange: "Group appends that arrive within 5 ms into one sync.",
				RegressionRisk: "Group commit changes the durability point of each event.",
				DeferralCost:   "None identified beyond the pilot.",
			},
			ExpectedBenefit:   "Roughly eight times the append throughput.",
			DeferralRationale: "The envelope's load is an eighth of the measured capacity.",
		},
		History: []core.FindingEvent{
			step(core.EventFindingOpened, 1, "P-0001", ""),
			step(core.EventEscalationRaised, 1, "P-0002", "the implementer disputes F-0002: the envelope's load is an eighth of the measured capacity, so REQ-9 is not at risk"),
			step(core.EventEscalationResolved, 1, "P-0003", "The latency budget holds at the envelope's load with a wide margin; F-0002 is an improvement, not a deficiency."),
			step(core.EventFindingReclassified, 1, "P-0003", "T1 to T2: TestLatencyBudget passes at 50 events per second with a 99th percentile of 40 ms."),
		},
	}
}

func f3Calibration() core.Finding {
	return core.Finding{
		ID: "F-0003", Status: core.StatusOpen, Tier: core.TierCalibration, Round: 1, Source: core.RoleReviewer,
		Input: core.FindingInput{
			Key: "backoff-cap", Tier: core.TierCalibration, Domain: core.DomainReliability,
			Title:          "The 5-minute backoff cap may delay recovery for long outages",
			Components:     []string{"internal/delivery/retry.go"},
			Description:    "After a long outage a recovered subscriber may wait up to five minutes for the next attempt.",
			Evidence:       "Follows from the backoff schedule in retry.go.",
			Impact:         "Slower catch-up after outages longer than ten minutes.",
			EvidenceNeeded: "Outage durations of pilot subscribers.",
			ReassessWhen:   "Two weeks of pilot traffic have been delivered.",
			Related:        []string{"F-0001"},
		},
		History: []core.FindingEvent{step(core.EventFindingOpened, 1, "P-0001", "")},
	}
}

// f4History is the history of F-0004 up to review P-0013; earlier packets see
// a prefix of it.
var f4History = []core.FindingEvent{
	step(core.EventFindingOpened, 1, "P-0001", ""),
	step(core.EventFindingFixClaimed, 1, "P-0002", "Retry the append when the sync fails."),
	step(core.EventFindingFixRejected, 1, "P-0004", "TestCrashRecovery still loses the acknowledged event in 2 of 10 runs with a kill between write and sync: the retry does not change when the acknowledgement is sent."),
	step(core.EventFindingFixClaimed, 1, "P-0005", "Sync before returning from append on the single-event endpoint."),
	step(core.EventFindingFixConfirmed, 1, "P-0006", "TestCrashRecovery passes 20 runs with a kill between write and sync."),
	step(core.EventFindingReopened, 2, "P-0007", "regression: the new batch endpoint acknowledges before the sync; TestCrashRecovery against /batch loses events in 4 of 10 runs."),
	step(core.EventFindingFixClaimed, 2, "P-0008", "Wait for the sync in the batch endpoint as well."),
	step(core.EventFindingFixRejected, 2, "P-0010", "The refactoring in the same commit moved the single-event acknowledgement before the sync again; TestCrashRecovery loses 1 of 10."),
	step(core.EventFindingFixClaimed, 2, "P-0011", "Acknowledge only after the append is synced."),
	step(core.EventFindingFixConfirmed, 2, "P-0012", "TestCrashRecovery passes 100 consecutive runs with a kill at every write boundary, on both endpoints."),
}

func f4(status core.FindingStatus, fix *core.Fix, steps int) core.Finding {
	return core.Finding{
		ID: "F-0004", Status: status, Tier: core.TierMaterial, Round: 1, Source: core.RoleReviewer,
		Recurrences: 3, RootCauseRequired: true, Fix: fix, History: f4History[:steps],
		Input: core.FindingInput{
			Key: "lost-on-crash", Tier: core.TierMaterial, Domain: core.DomainDataIntegrity,
			Title:       "An event acknowledged before its append is synced can be lost on a crash",
			Components:  []string{"internal/store/append.go", "internal/delivery/accept.go"},
			Description: "The HTTP handler acknowledges an event as soon as the append call returns, but the append returns before the sync completes.",
			Evidence:    "TestCrashRecovery with a kill between write and sync loses the acknowledged event in 3 of 10 runs.",
			Impact:      "An acknowledged event disappears after a restart, and the sender will not resend it.",
			Requirement: "REQ-4", BlockingRationale: "Restarts are inside the envelope; the pilot's senders rely on acknowledgement.",
			Verification: "TestCrashRecovery passes 100 consecutive runs with a kill at every write boundary.",
		},
	}
}

var f5History = []core.FindingEvent{
	step(core.EventFindingOpened, 2, "P-0007", ""),
	step(core.EventEscalationRaised, 2, "P-0008", "the implementer disputes F-0005: the replay tool is not part of the pilot, so the dependency direction blocks nothing at this stage"),
	step(core.EventEscalationResolved, 2, "P-0009", "Upheld: REQ-3 requires that internal/store import neither of the other packages, whether or not the replay tool ships."),
	step(core.EventFindingFixClaimed, 2, "P-0011", "Moved Event to internal/event."),
	step(core.EventFindingFixConfirmed, 2, "P-0012", "`go list -deps ./internal/store` no longer lists internal/delivery."),
}

func f5(status core.FindingStatus, fix *core.Fix, steps int) core.Finding {
	return core.Finding{
		ID: "F-0005", Status: status, Tier: core.TierMaterial, Round: 2, Source: core.RoleReviewer, Disputed: true,
		Fix: fix, History: f5History[:steps],
		Input: core.FindingInput{
			Key: "store-imports-delivery", Tier: core.TierMaterial, Domain: core.DomainArchitecture,
			Title:       "internal/store imports internal/delivery for the event type",
			Components:  []string{"internal/store/event.go"},
			Description: "The store depends on the delivery package for the Event type, which reverses the intended dependency direction.",
			Evidence:    "`go list -deps ./internal/store` includes internal/delivery.",
			Impact:      "Changes to delivery force rebuilding and retesting the store, and the store cannot be reused by the planned replay tool.",
			Requirement: "REQ-3", BlockingRationale: "The pilot adds the replay tool, which needs the store without delivery.",
			Verification: "`go list -deps ./internal/store` no longer lists internal/delivery.",
			Proportionality: &core.Proportionality{
				ProposedChange: "Move the Event type to a new internal/event package used by both.",
				RegressionRisk: "Mechanical; the compiler finds every use.",
				DeferralCost:   "Every package added during the pilot would deepen the inverted dependency.",
			},
		},
	}
}

func f6Preference() core.Finding {
	return core.Finding{
		ID: "F-0006", Status: core.StatusOpen, Tier: core.TierPreference, Round: 2, Source: core.RoleReviewer,
		Input: core.FindingInput{
			Key: "options-struct", Tier: core.TierPreference, Domain: core.DomainSimplicity,
			Title:       "Constructor takes seven positional parameters",
			Components:  []string{"internal/delivery/dispatcher.go"},
			Description: "An options struct would make call sites easier to read.",
			Evidence:    "dispatcher.go, NewDispatcher.",
			Impact:      "Readability only.",
		},
		History: []core.FindingEvent{step(core.EventFindingOpened, 2, "P-0007", "")},
	}
}

func f7Withdrawn() core.Finding {
	return core.Finding{
		ID: "F-0007", Status: core.StatusWithdrawn, Tier: core.TierCritical, Round: 2, Source: core.RoleReviewer, Disputed: true,
		Input: core.FindingInput{
			Key: "secret-in-log", Tier: core.TierCritical, Domain: core.DomainSecurityPrivacy,
			Title:       "Signing error message includes the subscriber secret",
			Components:  []string{"internal/delivery/sign.go"},
			Description: "The error returned when signing fails formats the key into the message.",
			Evidence:    "sign.go formats the key with %q in the error path.",
			Impact:      "A secret could reach the logs.",
			Risk:        "RISK-2", BlockingRationale: "Secrets in logs are an unacceptable risk at every stage.",
			Verification: "No error path in sign.go formats the key.",
		},
		History: []core.FindingEvent{
			step(core.EventFindingOpened, 2, "P-0007", ""),
			step(core.EventEscalationRaised, 2, "P-0008", "the implementer disputes F-0007: the error path is unreachable, since NewSigner rejects every key that could make signing fail"),
			step(core.EventEscalationResolved, 2, "P-0009", "Withdrawn: the only caller passes keys validated by NewSigner, and HMAC signing cannot fail for a valid key."),
			step(core.EventFindingWithdrawn, 2, "P-0009", "The error path cannot be reached with any key NewSigner accepts; the mechanism does not occur."),
		},
	}
}

func history(rounds int) []core.RoundSummary {
	commits := []string{commitRound1, commitRound2, commitFixed, commitFixed, commitRound5}
	digests := []string{digestRound1, digestRound2, digestFixed, digestFixed, digestRound5}
	verdicts := []core.Verdict{core.VerdictRed, core.VerdictRed, core.VerdictIndeterminate, "", core.VerdictRed}
	var out []core.RoundSummary
	for i := 0; i < rounds; i++ {
		s := core.RoundSummary{Round: i + 1, Candidate: core.Candidate{Commit: commits[i], Digest: digests[i]}, Verdict: verdicts[i]}
		if s.Verdict != "" {
			s.Reviewer = "review-agent"
		}
		out = append(out, s)
	}
	return out
}

func decisions() []core.Decision {
	made := func(d core.Decision) core.Decision {
		d.Stage, d.Contract = core.StagePilot, contractDigest
		return d
	}
	return []core.Decision{
		made(core.Decision{ID: "D-0001", Source: core.DecisionResolution, Escalation: "E-0001", Kind: core.EscalationContractGap, By: core.RoleGovernance,
			Candidate: digestRound1,
			Text:      "Dismissed: retention of delivered events is out of scope for the pilot; see the non-objectives."}),
		made(core.Decision{ID: "D-0003", Source: core.DecisionDirective, Packet: "P-0003", By: core.RoleAuditor, Candidate: digestFirst,
			Text: "Durability findings are verified with TestCrashRecovery at every write boundary, not with the happy-path store tests."}),
		made(core.Decision{ID: "D-0004", Source: core.DecisionResolution, Escalation: "E-0002", Kind: core.EscalationDispute, Packet: "P-0003", By: core.RoleAuditor,
			Candidate: digestFirst,
			Text:      "The latency budget holds at the envelope's load with a wide margin; F-0002 is an improvement, not a deficiency."}),
		made(core.Decision{ID: "D-0005", Source: core.DecisionResolution, Escalation: "E-0003", Kind: core.EscalationDispute, Packet: "P-0009", By: core.RoleAuditor,
			Candidate: digestBatch,
			Text:      "Upheld: REQ-3 requires that internal/store import neither of the other packages, whether or not the replay tool ships."}),
		made(core.Decision{ID: "D-0006", Source: core.DecisionResolution, Escalation: "E-0004", Kind: core.EscalationDispute, Packet: "P-0009", By: core.RoleAuditor,
			Candidate: digestBatch,
			Text:      "Withdrawn: the only caller passes keys validated by NewSigner, and HMAC signing cannot fail for a valid key."}),
	}
}

// earlierDecisions are decisions that no longer bind, one for each reason: a
// directive an audit superseded, a directive issued under an earlier
// contract, and evidence governance supplied for another candidate.
func earlierDecisions() []core.Decision {
	return []core.Decision{
		{ID: "D-0002", Source: core.DecisionDirective, Packet: "P-0003", By: core.RoleAuditor, Stage: core.StagePilot, Contract: contractDigest,
			Candidate: digestFirst, SupersededBy: "P-0009",
			Text: "Treat every finding about the replay tool as outside the pilot."},
		{ID: "D-0007", Source: core.DecisionDirective, Packet: "P-0009", By: core.RoleAuditor, Stage: core.StageControlledValidation, Contract: earlierContract,
			Candidate: digestBatch,
			Text:      "Verify retry behaviour against the 10-minute outage of the validation plan."},
		{ID: "D-0008", Source: core.DecisionResolution, Escalation: "E-0005", Kind: core.EscalationEvidenceGap, By: core.RoleGovernance,
			Stage: core.StagePilot, Contract: contractDigest, Candidate: digestRound1, Requirements: []string{"REQ-7"},
			Text: "REQ-7 holds: the one-hour soak test of 14 November stayed within 6% of the five-minute figure."},
	}
}

func base(id string, action core.Step, round int, candidate core.Candidate) core.Packet {
	role, _ := action.PacketRole()
	c := fullContract()
	p := core.Packet{
		Schema: core.SchemaPacket, ID: id, Protocol: core.ProtocolVersion,
		Role: role, Action: action, Round: round, IssuedAt: "2026-11-20T14:03:09Z",
		Candidate: candidate, ReadOnly: role.ReadOnly(),
		ContractDigest: contractDigest, Contract: c, ApplicableRequirements: applicableIDs(c),
		ReportSchema: core.SchemaReport, Decisions: decisions(), EarlierDecisions: earlierDecisions(),
	}
	// A review packet names no round and carries no earlier rounds.
	if action == core.StepReview {
		p.Round = 0
	}
	return p
}

// established fills in the contract the findings were established under:
// the pinned one, except F-0001, which predates it.
func established(p core.Packet) core.Packet {
	for i := range p.Findings {
		// A review packet's findings come without rounds.
		if p.Action == core.StepReview {
			p.Findings[i].Round = 0
			p.Findings[i].History = slices.Clone(p.Findings[i].History)
			for j := range p.Findings[i].History {
				p.Findings[i].History[j].Round = 0
			}
		}
		p.Findings[i].Contract = contractDigest
		if p.Findings[i].ID == "F-0001" && p.Contract.Project.Name == "Courier" {
			p.Findings[i].Contract = earlierContract
		}
	}
	return p
}

func fixF4() *core.Fix {
	return &core.Fix{Finding: "F-0004", Commits: []string{commitFixed}, Summary: "Acknowledge only after the append is synced.",
		Components: []string{"internal/delivery/accept.go", "internal/store/append.go"},
		RootCause:  "Both endpoints treated the return of append as durability, and every earlier fix patched one acknowledgement path. Acknowledgement now goes through a single function that waits for the sync, and the retries added in P-0002 are removed.",
		Tests:      "TestCrashRecovery now kills the process at every write boundary on both endpoints; 100 runs pass."}
}

func fixF5() *core.Fix {
	return &core.Fix{Finding: "F-0005", Commits: []string{commitFixed}, Summary: "Moved Event to internal/event.",
		Components: []string{"internal/event", "internal/store", "internal/delivery"}}
}

func fullReview() core.Packet {
	p := base("P-0013", core.StepReview, 3, core.Candidate{Commit: commitFixed, Digest: digestFixed})
	p.Evidence = []core.Evidence{
		passed("REQ-1", "go test ./internal/delivery -run TestAtLeastOnce", digestFixed),
		passed("REQ-4", "go test ./internal/store -run TestCrashRecovery", digestFixed),
		passed("REQ-8", "go test ./internal/delivery -run TestSubscriberRecovery", digestFixed),
		passed("REQ-9", "go test ./internal/delivery -run TestLatencyBudget", digestFixed),
	}
	p.Findings = []core.Finding{
		f1Resolved(), f2Reclassified(), f3Calibration(),
		f4(core.StatusResolved, fixF4(), len(f4History)), f5(core.StatusResolved, fixF5(), len(f5History)),
		f6Preference(), f7Withdrawn(),
	}
	p.Verified = &core.Candidate{Commit: commitFixed, Digest: digestFixed}
	return p
}

func fullFix() core.Packet {
	p := base("P-0011", core.StepFix, 2, core.Candidate{Commit: commitBatch, Digest: digestBatch})
	p.Base = &core.Candidate{Commit: commitRound2, Digest: digestRound2}
	p.Evidence = []core.Evidence{
		passed("REQ-1", "go test ./internal/delivery -run TestAtLeastOnce", digestBatch),
		passed("REQ-4", "go test ./internal/store -run TestCrashRecovery", digestBatch),
		passed("REQ-8", "go test ./internal/delivery -run TestSubscriberRecovery", digestBatch),
		passed("REQ-9", "go test ./internal/delivery -run TestLatencyBudget", digestBatch),
	}
	p.Subjects = []string{"F-0004", "F-0005"}
	p.RootCauseRequired = []string{"F-0004"}
	rejected := &core.Fix{Finding: "F-0004", Commits: []string{commitBatch}, Summary: "Wait for the sync in the batch endpoint as well.",
		Components: []string{"internal/delivery/batch.go", "internal/delivery/accept.go"}}
	p.Findings = []core.Finding{f4(core.StatusOpen, rejected, 8), f5(core.StatusOpen, nil, 3)}
	p.History = history(2)
	return p
}

func fullVerify() core.Packet {
	p := base("P-0012", core.StepVerify, 2, core.Candidate{Commit: commitFixed, Digest: digestFixed})
	p.Base = &core.Candidate{Commit: commitRound2, Digest: digestRound2}
	p.Subjects = []string{"F-0004", "F-0005"}
	p.Findings = []core.Finding{f4(core.StatusClaimed, fixF4(), 9), f5(core.StatusClaimed, fixF5(), 4)}
	p.History = history(2)
	return p
}

func fullAudit() core.Packet {
	p := base("P-0021", core.StepAudit, 5, core.Candidate{Commit: commitRound5, Digest: digestRound5})
	p.Evidence = []core.Evidence{
		passed("REQ-1", "go test ./internal/delivery -run TestAtLeastOnce", digestRound5),
		passed("REQ-4", "go test ./internal/store -run TestCrashRecovery -count 1", digestRound5),
		passed("REQ-9", "go test ./internal/delivery -run TestLatencyBudget", digestRound5),
	}
	disputed := core.Finding{
		ID: "F-0008", Status: core.StatusOpen, Tier: core.TierMaterial, Round: 4, Source: core.RoleReviewer, Disputed: true,
		Input: core.FindingInput{
			Key: "shared-client", Tier: core.TierMaterial, Domain: core.DomainPerformance,
			Title:       "All deliveries share an HTTP client limited to two connections",
			Components:  []string{"internal/delivery/client.go"},
			Description: "The shared client allows two connections, so deliveries to fast subscribers queue behind slow ones.",
			Evidence:    "With 20 subscribers of which 2 respond in 2 s, TestLatencyBudget measures a 99th percentile of 1.8 s.",
			Impact:      "Latency exceeds the 500 ms budget whenever two subscribers are slow.",
			Requirement: "REQ-9", BlockingRationale: "Two of the pilot's subscribers are batch systems known to respond slowly.",
			Verification: "TestLatencyBudget with two 2-second subscribers stays under 500 ms at the 99th percentile.",
			Proportionality: &core.Proportionality{
				ProposedChange: "Size the connection pool to the number of subscribers.",
				RegressionRisk: "More open connections, bounded by the envelope's 200 subscribers.",
				DeferralCost:   "Latency failures during the pilot would be blamed on subscribers and hide the cause.",
			},
		},
		History: []core.FindingEvent{
			step(core.EventFindingOpened, 4, "P-0017", ""),
			step(core.EventEscalationRaised, 5, "P-0020", "the implementer disputes F-0008: the envelope says nothing about subscriber latency, so slow subscribers are outside it"),
		},
	}
	contradicting := core.Finding{
		ID: "F-0009", Status: core.StatusOpen, Tier: core.TierMaterial, Round: 5, Source: core.RoleReviewer,
		Input: core.FindingInput{
			Key: "retry-unbounded", Tier: core.TierMaterial, Domain: core.DomainStability,
			Title:       "Retry state for a dead subscriber grows for an hour",
			Components:  []string{"internal/delivery/retry.go"},
			Description: "Retrying for one hour keeps every pending event of a dead subscriber in memory for that hour.",
			Evidence:    "At 50 events per second for one subscriber that never recovers, resident memory grows by 600 MB in an hour.",
			Impact:      "Memory is not bounded under sustained load with one dead subscriber, which is inside the envelope.",
			Requirement: "REQ-7", BlockingRationale: "A dead subscriber during the pilot would exhaust the instance's memory.",
			Verification: "The REQ-7 soak test passes with one subscriber that never recovers.",
			Contradicts:  "F-0001",
		},
		History: []core.FindingEvent{
			step(core.EventFindingOpened, 5, "P-0019", ""),
			step(core.EventEscalationRaised, 5, "P-0019", "F-0009 declares that it reverses the resolution of F-0001"),
		},
	}
	p.Escalations = []core.Escalation{
		{ID: "E-0006", Kind: core.EscalationDispute, Owner: core.RoleAuditor, Round: 5, Open: true,
			Reason:   "the implementer disputes F-0008: the envelope says nothing about subscriber latency, so slow subscribers are outside it",
			Findings: []string{"F-0008"}},
		{ID: "E-0007", Kind: core.EscalationContradiction, Owner: core.RoleAuditor, Round: 5, Open: true,
			Reason:   "F-0009 declares that it reverses the resolution of F-0001",
			Findings: []string{"F-0009"}},
		{ID: "E-0008", Kind: core.EscalationComponentChurn, Owner: core.RoleAuditor, Round: 5, Open: true,
			Reason:     "component \"internal/delivery/retry.go\" was named in fixes in 3 distinct rounds of stage pilot",
			Findings:   []string{"F-0001"},
			Components: []string{"internal/delivery/retry.go"}},
		{ID: "E-0009", Kind: core.EscalationRegressionSpiral, Owner: core.RoleAuditor, Round: 4, Open: true,
			Reason: "verification P-0018 found 2 blocking regressions and confirmed 1 fixes"},
		{ID: "E-0010", Kind: core.EscalationProcessAudit, Owner: core.RoleAuditor, Round: 5, Open: true,
			Reason: "5 consecutive rounds of stage pilot ended without GREEN"},
	}
	p.Adjudicable = []string{"F-0008", "F-0009"}
	p.Findings = []core.Finding{f1Resolved(), f2Reclassified(), f3Calibration(), disputed, contradicting}
	p.History = history(5)
	return p
}

// Minimal packets: the least each action can carry.

func minimalContract() core.Contract {
	return core.Contract{
		Schema:       core.SchemaContract,
		Project:      core.Project{Name: "Minimal", Objectives: []string{"Exist."}},
		Stage:        core.StagePrototype,
		Authorizes:   "Internal demonstration.",
		Scope:        []string{"."},
		Envelope:     []string{"One user."},
		Requirements: []core.Requirement{{ID: "REQ-1", Domain: core.DomainFunctionalCorrectness, Statement: "It runs.", Criterion: "It exits 0."}},
		Risks:        core.Risks{Unacceptable: []core.Risk{{ID: "RISK-1", Statement: "Data loss."}}},
	}
}

func minimal(action core.Step) core.Packet {
	role, _ := action.PacketRole()
	p := core.Packet{
		Schema: core.SchemaPacket, ID: "P-0001", Protocol: core.ProtocolVersion,
		Role: role, Action: action, Round: map[bool]int{true: 0, false: 1}[action == core.StepReview], IssuedAt: "2026-11-20T09:00:00Z",
		Candidate: core.Candidate{Commit: commitRound1, Digest: digestRound1}, ReadOnly: role.ReadOnly(),
		ContractDigest: contractDigest, Contract: minimalContract(), ApplicableRequirements: []string{"REQ-1"},
		ReportSchema: core.SchemaReport,
	}
	finding := core.Finding{ID: "F-0001", Status: core.StatusOpen, Tier: core.TierCritical, Round: 1, Source: core.RoleReviewer, Contract: contractDigest,
		Input: core.FindingInput{Key: "k", Tier: core.TierCritical, Domain: core.DomainFunctionalCorrectness, Title: "Crashes on start",
			Components: []string{"main.go"}, Description: "d", Evidence: "e", Impact: "i"}}
	switch action {
	case core.StepFix:
		p.Base = &p.Candidate
		p.Subjects = []string{"F-0001"}
		p.Findings = []core.Finding{finding}
	case core.StepVerify:
		p.Base = &core.Candidate{Commit: commitRound2, Digest: digestRound2}
		finding.Status = core.StatusClaimed
		finding.Fix = &core.Fix{Finding: "F-0001", Commits: []string{commitRound1}, Summary: "s", Components: []string{"main.go"}}
		p.Subjects = []string{"F-0001"}
		p.Findings = []core.Finding{finding}
	case core.StepAudit:
		p.Escalations = []core.Escalation{{ID: "E-0001", Kind: core.EscalationNonConvergence, Owner: core.RoleAuditor,
			Round: 3, Reason: "Three consecutive rounds have ended without GREEN.", Open: true}}
	}
	return p
}

var actions = []core.Step{core.StepReview, core.StepFix, core.StepVerify, core.StepAudit}

func full(action core.Step) core.Packet {
	switch action {
	case core.StepReview:
		return established(fullReview())
	case core.StepFix:
		return established(fullFix())
	case core.StepVerify:
		return established(fullVerify())
	}
	return established(fullAudit())
}
